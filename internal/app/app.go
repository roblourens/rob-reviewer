package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roblourens/rob-reviewer/internal/config"
	reviewcopilot "github.com/roblourens/rob-reviewer/internal/copilot"
	"github.com/roblourens/rob-reviewer/internal/diff"
	"github.com/roblourens/rob-reviewer/internal/focus"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/poller"
	"github.com/roblourens/rob-reviewer/internal/review"
	"github.com/roblourens/rob-reviewer/internal/source"
	"github.com/roblourens/rob-reviewer/internal/state"
	"github.com/roblourens/rob-reviewer/internal/workspace"
)

type App struct {
	config        config.Config
	logger        *slog.Logger
	reviewClient  *github.Client
	stateClient   *github.Client
	focusCatalog  *focus.Catalog
	copilotToken  string
	hasStateToken bool

	runnerMutex sync.Mutex
	runner      *reviewcopilot.Runner
}

type Options struct {
	ConfigPath   string
	Logger       *slog.Logger
	StateToken   string
	ReviewToken  string
	CopilotToken string
}

type PollResult struct {
	Poll    poller.Result
	Reviews []review.Result
}

func New(options Options) (*App, error) {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	cfg, err := config.Load(options.ConfigPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.ReviewToken) == "" {
		return nil, errors.New("REVIEW_GITHUB_TOKEN is required")
	}
	if strings.TrimSpace(options.CopilotToken) == "" {
		return nil, errors.New("COPILOT_GITHUB_TOKEN is required")
	}

	configDirectory, err := filepath.Abs(filepath.Dir(options.ConfigPath))
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}
	focusCatalog, err := focus.Load(filepath.Join(configDirectory, "reviewers"))
	if err != nil {
		return nil, err
	}
	if _, err := focusCatalog.Resolve(cfg.Review.Focuses); err != nil {
		return nil, err
	}

	return &App{
		config:        cfg,
		logger:        options.Logger,
		reviewClient:  github.NewClient(nil, options.ReviewToken),
		stateClient:   github.NewClient(nil, options.StateToken),
		focusCatalog:  focusCatalog,
		copilotToken:  options.CopilotToken,
		hasStateToken: strings.TrimSpace(options.StateToken) != "",
	}, nil
}

func (app *App) Close() error {
	app.runnerMutex.Lock()
	defer app.runnerMutex.Unlock()
	if app.runner == nil {
		return nil
	}
	err := app.runner.Close()
	app.runner = nil
	return err
}

func (app *App) Poll(ctx context.Context, stateRepository, outputDirectory string) (PollResult, error) {
	if !app.config.Automation.Enabled {
		app.logger.Info("automatic reviewer is disabled by configuration")
		return PollResult{}, nil
	}
	stateOwner, stateRepo, err := parseRepository(stateRepository)
	if err != nil {
		return PollResult{}, err
	}
	if !app.hasStateToken {
		return PollResult{}, errors.New("GITHUB_TOKEN is required for polling state")
	}
	if strings.TrimSpace(outputDirectory) == "" {
		return PollResult{}, errors.New("poll output directory is required so review state cannot advance without durable reports")
	}
	store := state.NewStore(
		app.stateClient,
		stateOwner,
		stateRepo,
		app.config.State.Branch,
		app.config.State.Path,
	)
	var reviews []review.Result
	prPoller := poller.New(
		app.reviewClient,
		store,
		poller.Options{
			Owner:         app.config.Target.Owner,
			Repo:          app.config.Target.Repo,
			MaxPerRun:     app.config.Poll.MaxPerRun,
			MaxPerDay:     app.config.Poll.MaxPerDay,
			QuietPeriod:   time.Duration(app.config.Poll.QuietMinutes) * time.Minute,
			ScanWindow:    time.Duration(app.config.Poll.ScanWindowHours) * time.Hour,
			MaxPendingAge: time.Duration(app.config.Poll.MaxPendingHours) * time.Hour,
			SkipLabels:    slices.Clone(app.config.Automation.SkipLabels),
		},
		func(reviewContext context.Context, pull review.PullRequest) (poller.ReviewOutcome, error) {
			if app.config.Publication.Mode == "automatic" {
				automaticPublisher := review.NewAutomaticPublisher(
					app.reviewClient,
					app.config.Target.Owner,
					app.config.Target.Repo,
					app.config.Automation.SkipLabels...,
				)
				existing, err := automaticPublisher.PrepareAutomaticReview(
					reviewContext,
					pull,
				)
				if err != nil {
					return poller.ReviewOutcome{}, fmt.Errorf("check automatic review marker: %w", err)
				}
				if existing != nil {
					if strings.EqualFold(existing.State, "PENDING") {
						err := automaticPublisher.ResumePending(reviewContext, pull, existing.ID)
						if err != nil {
							if errors.Is(err, review.ErrPublicationSuppressed) {
								return poller.ReviewOutcome{}, nil
							}
							return poller.ReviewOutcome{}, err
						}
						return poller.ReviewOutcome{Published: true}, nil
					}
					return poller.ReviewOutcome{}, nil
				}
			}
			result, analyzed, err := app.reviewOne(reviewContext, pull, false)
			if err != nil {
				return poller.ReviewOutcome{}, err
			}
			if !analyzed {
				return poller.ReviewOutcome{}, nil
			}
			if _, err := WriteResultFiles(outputDirectory, result); err != nil {
				return poller.ReviewOutcome{}, fmt.Errorf("persist dry-run report for PR %d: %w", pull.Number, err)
			}
			reviews = append(reviews, result)
			if app.config.Publication.Mode != "automatic" || len(result.Findings) == 0 {
				return poller.ReviewOutcome{}, nil
			}
			published, err := review.NewAutomaticPublisher(
				app.reviewClient,
				app.config.Target.Owner,
				app.config.Target.Repo,
				app.config.Automation.SkipLabels...,
			).Publish(reviewContext, result, false)
			if err != nil {
				if errors.Is(err, review.ErrPublicationSuppressed) {
					return poller.ReviewOutcome{}, nil
				}
				return poller.ReviewOutcome{}, err
			}
			return poller.ReviewOutcome{Published: published}, nil
		},
	)
	pollResult, err := prPoller.Run(ctx)
	if err != nil {
		return PollResult{Poll: pollResult, Reviews: reviews}, err
	}
	return PollResult{Poll: pollResult, Reviews: reviews}, nil
}

func (app *App) ReviewPullRequest(ctx context.Context, number int, publish bool) (review.Result, bool, error) {
	if number < 1 {
		return review.Result{}, false, errors.New("pull request number must be positive")
	}
	pull, err := app.reviewClient.GetPullRequest(ctx, app.config.Target.Owner, app.config.Target.Repo, number)
	if err != nil {
		return review.Result{}, false, fmt.Errorf("get pull request %d: %w", number, err)
	}
	if !strings.EqualFold(pull.State, "open") {
		return review.Result{}, false, fmt.Errorf("pull request %d is not open", number)
	}
	if !pull.IsTeamAuthored() {
		return review.Result{}, false, fmt.Errorf(
			"pull request %d was opened by %q with author association %q; only team-authored PRs are eligible",
			number,
			pull.AuthorLogin,
			pull.AuthorAssociation,
		)
	}
	if pull.Draft {
		return review.Result{}, false, fmt.Errorf("pull request %d is still a draft", number)
	}
	return app.reviewOne(ctx, pull, publish)
}

func (app *App) ReviewHistoricalPullRequest(ctx context.Context, number int) (review.Result, error) {
	if number < 1 {
		return review.Result{}, errors.New("pull request number must be positive")
	}
	pull, err := app.reviewClient.GetPullRequest(ctx, app.config.Target.Owner, app.config.Target.Repo, number)
	if err != nil {
		return review.Result{}, fmt.Errorf("get historical pull request %d: %w", number, err)
	}
	if !pull.IsTeamAuthored() {
		return review.Result{}, fmt.Errorf(
			"historical pull request %d was opened by %q with author association %q; only team-authored PRs are eligible",
			number,
			pull.AuthorLogin,
			pull.AuthorAssociation,
		)
	}
	if pull.Draft {
		return review.Result{}, fmt.Errorf("historical pull request %d is still a draft", number)
	}

	result, err := app.analyze(ctx, pull)
	if err != nil {
		return review.Result{}, err
	}
	app.logger.Info(
		"historical pull request review complete",
		"pr", pull.Number,
		"head", pull.HeadSHA,
		"findings", len(result.Findings),
		"published", false,
		"model", result.Stats.Model,
		"actualModels", result.Stats.ActualModels,
		"reasoningEffort", result.Stats.ReasoningEffort,
		"actualReasoningEfforts", result.Stats.ActualReasoningEfforts,
		"apiEndpoints", result.Stats.APIEndpoints,
		"wallClockMilliseconds", result.Stats.WallClockMilliseconds,
		"modelCalls", result.Stats.ModelCalls,
		"inputTokens", result.Stats.InputTokens,
		"outputTokens", result.Stats.OutputTokens,
		"totalTokens", result.Stats.TotalTokens,
		"reasoningTokens", result.Stats.ReasoningTokens,
		"cacheReadTokens", result.Stats.CacheReadTokens,
		"cacheWriteTokens", result.Stats.CacheWriteTokens,
		"billingTokensByType", result.Stats.BillingTokensByType,
		"apiDurationMilliseconds", result.Stats.APIDurationMilliseconds,
		"toolCalls", result.Stats.ToolCalls,
		"nanoAIUnits", result.Stats.NanoAIUnits,
		"modelBillingMultipliers", result.Stats.ModelBillingMultipliers,
		"costNote", result.Stats.CostNote,
	)
	return result, nil
}

func (app *App) reviewOne(ctx context.Context, pull review.PullRequest, publish bool) (_ review.Result, analyzed bool, returnErr error) {
	marker := github.ReviewMarker(pull.Number, pull.HeadSHA)
	existing, err := app.reviewClient.FindReviewMarker(ctx, app.config.Target.Owner, app.config.Target.Repo, pull.Number, marker)
	if err != nil {
		return review.Result{}, false, fmt.Errorf("check review idempotency marker: %w", err)
	}
	if existing != nil && !strings.EqualFold(existing.State, "PENDING") {
		app.logger.Info("pull request already reviewed", "pr", pull.Number, "head", pull.HeadSHA)
		return review.Result{PullRequest: pull}, false, nil
	}

	result, err := app.analyze(ctx, pull)
	if err != nil {
		return review.Result{}, false, err
	}
	publisher := review.NewPublisher(
		app.reviewClient,
		app.config.Target.Owner,
		app.config.Target.Repo,
		app.config.Automation.SkipLabels...,
	)
	published, err := publisher.Publish(ctx, result, !publish)
	if err != nil {
		return review.Result{}, false, err
	}
	app.logger.Info(
		"pull request review complete",
		"pr", pull.Number,
		"head", pull.HeadSHA,
		"findings", len(result.Findings),
		"published", published,
		"model", result.Stats.Model,
		"actualModels", result.Stats.ActualModels,
		"reasoningEffort", result.Stats.ReasoningEffort,
		"actualReasoningEfforts", result.Stats.ActualReasoningEfforts,
		"apiEndpoints", result.Stats.APIEndpoints,
		"wallClockMilliseconds", result.Stats.WallClockMilliseconds,
		"modelCalls", result.Stats.ModelCalls,
		"inputTokens", result.Stats.InputTokens,
		"outputTokens", result.Stats.OutputTokens,
		"totalTokens", result.Stats.TotalTokens,
		"reasoningTokens", result.Stats.ReasoningTokens,
		"cacheReadTokens", result.Stats.CacheReadTokens,
		"cacheWriteTokens", result.Stats.CacheWriteTokens,
		"billingTokensByType", result.Stats.BillingTokensByType,
		"apiDurationMilliseconds", result.Stats.APIDurationMilliseconds,
		"toolCalls", result.Stats.ToolCalls,
		"nanoAIUnits", result.Stats.NanoAIUnits,
		"modelBillingMultipliers", result.Stats.ModelBillingMultipliers,
		"costNote", result.Stats.CostNote,
	)
	return result, true, nil
}

func (app *App) analyze(ctx context.Context, pull review.PullRequest) (_ review.Result, returnErr error) {
	startedAt := time.Now()
	checkout, err := workspace.New(ctx, app.config.Target.Owner, app.config.Target.Repo, pull)
	if err != nil {
		return review.Result{}, err
	}
	defer func() {
		if err := checkout.Close(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close pull request workspace: %w", err))
		}
	}()

	parsedDiff, err := diff.Parse(checkout.Diff)
	if err != nil {
		return review.Result{}, fmt.Errorf("parse pull request diff: %w", err)
	}
	reviewSource, err := source.New(checkout.Root, pull, parsedDiff, app.focusCatalog)
	if err != nil {
		return review.Result{}, err
	}
	runner, err := app.getRunner(ctx)
	if err != nil {
		return review.Result{}, err
	}
	findings, analysis, stats, err := runner.Review(ctx, reviewSource)
	if err != nil {
		return review.Result{}, err
	}
	completedAt := time.Now()
	stats.StartedAt = startedAt.UTC()
	stats.CompletedAt = completedAt.UTC()
	stats.WallClockMilliseconds = completedAt.Sub(startedAt).Milliseconds()
	result := review.Result{PullRequest: pull, Findings: findings, Analysis: analysis, Stats: stats}
	review.BindFindingIDs(&result)
	return result, nil
}

func (app *App) getRunner(ctx context.Context) (*reviewcopilot.Runner, error) {
	app.runnerMutex.Lock()
	defer app.runnerMutex.Unlock()
	if app.runner != nil {
		return app.runner, nil
	}
	runner, err := reviewcopilot.NewRunner(ctx, reviewcopilot.Options{
		GitHubToken:     app.copilotToken,
		Model:           app.config.Review.Model,
		ReasoningEffort: app.config.Review.ReasoningEffort,
		FocusRoot:       app.focusCatalog.Root(),
		FocusNames:      app.config.Review.Focuses,
		MinConfidence:   app.config.Review.MinConfidence,
		MaxFindings:     app.config.Review.MaxFindings,
	})
	if err != nil {
		return nil, err
	}
	app.runner = runner
	return runner, nil
}

func PrintResult(result review.Result) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("print review result: %w", err)
	}
	return nil
}

func PrintResultMarkdown(result review.Result) error {
	if _, err := fmt.Fprint(os.Stdout, FormatResultMarkdown(result)); err != nil {
		return fmt.Errorf("print Markdown review result: %w", err)
	}
	return nil
}

func FormatResultMarkdown(result review.Result) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# Performance review: %s\n\n", singleLineMarkdown(result.PullRequest.Title))
	fmt.Fprintf(&output, "[PR #%d](%s) at `%s`\n\n", result.PullRequest.Number, result.PullRequest.URL, result.PullRequest.HeadSHA)
	if len(result.Findings) == 0 {
		output.WriteString("## Findings\n\nNo high-confidence performance findings.\n")
	} else {
		fmt.Fprintf(&output, "## Findings (%d)\n\n", len(result.Findings))
		for index, finding := range result.Findings {
			fmt.Fprintf(
				&output,
				"### %d. [%s] %s\n\n**Finding ID:** `%s`  \n**Location:** <code>%s:%d</code> (<code>%s</code>)  \n**Performance category:** <code>%s</code>  \n**Resource:** %s  \n**Scaling:** %s  \n**Outcome:** %s  \n**PR causality:** <code>%s</code>  \n**Previous behavior:** %s  \n**Changed behavior:** %s  \n**Confidence:** %.2f — %s\n\n**Causal diff evidence:** %s\n\n%s\n\n**Evidence:** %s\n\n**Suggested direction:** %s\n\n",
				index+1,
				finding.Severity,
				singleLineMarkdown(finding.Title),
				review.SanitizeMarkdownText(finding.ID),
				review.SanitizeMarkdownText(finding.Path),
				finding.Line,
				finding.Side,
				review.SanitizeMarkdownText(finding.PerformanceCategory),
				review.SanitizeMarkdownText(finding.PerformanceResource),
				review.SanitizeMarkdownText(finding.PerformanceScaling),
				review.SanitizeMarkdownText(finding.PerformanceOutcome),
				review.SanitizeMarkdownText(finding.ChangeCausality),
				review.SanitizeMarkdownText(finding.PreviousBehavior),
				review.SanitizeMarkdownText(finding.ChangedBehavior),
				finding.Confidence,
				review.SanitizeMarkdownText(finding.ConfidenceRationale),
				review.SanitizeMarkdownText(finding.CausalDiffEvidence),
				review.SanitizeMarkdownText(finding.Impact),
				review.SanitizeMarkdownText(finding.Evidence),
				review.SanitizeMarkdownText(finding.Recommendation),
			)
		}
	}
	if len(result.Analysis.Scenarios) > 0 {
		output.WriteString("\n<details>\n<summary>Reviewer scenario analysis</summary>\n\n")
		if len(result.Analysis.RelevantIssueFamilies) > 0 {
			fmt.Fprintf(&output, "**Relevant issue families:** %s\n\n", review.SanitizeMarkdownText(strings.Join(result.Analysis.RelevantIssueFamilies, ", ")))
		}
		for index, scenario := range result.Analysis.Scenarios {
			fmt.Fprintf(&output, "### Scenario %d: %s\n\n", index+1, singleLineMarkdown(scenario.Scenario))
			fmt.Fprintf(&output, "- Critical path: %s\n", review.SanitizeMarkdownText(scenario.CriticalPath))
			fmt.Fprintf(&output, "- Scaling input: %s\n", review.SanitizeMarkdownText(scenario.ScalingInput))
			fmt.Fprintf(&output, "- Effective concurrency: %s\n", review.SanitizeMarkdownText(scenario.EffectiveConcurrency))
			fmt.Fprintf(&output, "- Cache behavior: %s\n", review.SanitizeMarkdownText(scenario.CacheBehavior))
			fmt.Fprintf(&output, "- Mechanism confidence: %s\n", review.SanitizeMarkdownText(scenario.MechanismConfidence))
			fmt.Fprintf(&output, "- Magnitude uncertainty: %s\n", review.SanitizeMarkdownText(scenario.MagnitudeUncertainty))
			if len(scenario.ExpensiveBoundaries) == 0 {
				output.WriteString("- Expensive boundaries: none found\n")
			} else {
				output.WriteString("- Expensive boundaries:\n")
				for _, boundary := range scenario.ExpensiveBoundaries {
					fmt.Fprintf(
						&output,
						"  - %s at <code>%s</code>: %s; cardinality: %s\n",
						review.SanitizeMarkdownText(boundary.Kind),
						review.SanitizeMarkdownText(boundary.Location),
						review.SanitizeMarkdownText(boundary.Operation),
						review.SanitizeMarkdownText(boundary.Cardinality),
					)
					fmt.Fprintf(
						&output,
						"    - Introduced by diff: %t; previous behavior: %s; critical-path effect: %s\n",
						boundary.IntroducedByDiff,
						review.SanitizeMarkdownText(boundary.PreviousBehavior),
						review.SanitizeMarkdownText(boundary.CriticalPathEffect),
					)
				}
			}
			fmt.Fprintf(&output, "- Verdict: %s\n\n", review.SanitizeMarkdownText(scenario.Verdict))
		}
		fmt.Fprintf(&output, "**Summary:** %s\n\n</details>\n", review.SanitizeMarkdownText(result.Analysis.Summary))
	}
	fmt.Fprintf(&output, "\n%s\n", review.FormatStatsMarkdown(result.Stats))
	return output.String()
}

func singleLineMarkdown(value string) string {
	return strings.ReplaceAll(review.SanitizeMarkdownText(value), "\n", " ")
}

func parseRepository(value string) (string, string, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("repository must be owner/name, got %q", value)
	}
	return parts[0], parts[1], nil
}

func ParsePullRequestNumber(value string) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid pull request number %q", value)
	}
	return number, nil
}
