package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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

func (app *App) Poll(ctx context.Context, stateRepository string) (poller.Result, error) {
	stateOwner, stateRepo, err := parseRepository(stateRepository)
	if err != nil {
		return poller.Result{}, err
	}
	if !app.hasStateToken {
		return poller.Result{}, errors.New("GITHUB_TOKEN is required for polling state")
	}
	store := state.NewStore(
		app.stateClient,
		stateOwner,
		stateRepo,
		app.config.State.Branch,
		app.config.State.Path,
	)
	prPoller := poller.New(
		app.reviewClient,
		store,
		app.config.Target.Owner,
		app.config.Target.Repo,
		app.config.Poll.MaxPerRun,
		func(reviewContext context.Context, pull review.PullRequest) error {
			_, err := app.reviewOne(reviewContext, pull, true)
			return err
		},
	)
	return prPoller.Run(ctx)
}

func (app *App) ReviewPullRequest(ctx context.Context, number int, publish bool) (review.Result, error) {
	if number < 1 {
		return review.Result{}, errors.New("pull request number must be positive")
	}
	pull, err := app.reviewClient.GetPullRequest(ctx, app.config.Target.Owner, app.config.Target.Repo, number)
	if err != nil {
		return review.Result{}, fmt.Errorf("get pull request %d: %w", number, err)
	}
	if !strings.EqualFold(pull.State, "open") {
		return review.Result{}, fmt.Errorf("pull request %d is not open", number)
	}
	if pull.Draft {
		return review.Result{}, fmt.Errorf("pull request %d is still a draft", number)
	}
	return app.reviewOne(ctx, pull, publish)
}

func (app *App) reviewOne(ctx context.Context, pull review.PullRequest, publish bool) (_ review.Result, returnErr error) {
	marker := github.ReviewMarker(pull.Number, pull.HeadSHA)
	exists, err := app.reviewClient.HasReviewMarker(ctx, app.config.Target.Owner, app.config.Target.Repo, pull.Number, marker)
	if err != nil {
		return review.Result{}, fmt.Errorf("check review idempotency marker: %w", err)
	}
	if exists {
		app.logger.Info("pull request already reviewed", "pr", pull.Number, "head", pull.HeadSHA)
		return review.Result{PullRequest: pull}, nil
	}

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
	findings, err := runner.Review(ctx, reviewSource)
	if err != nil {
		return review.Result{}, err
	}
	result := review.Result{PullRequest: pull, Findings: findings}
	publisher := review.NewPublisher(app.reviewClient, app.config.Target.Owner, app.config.Target.Repo)
	published, err := publisher.Publish(ctx, result, !publish)
	if err != nil {
		return review.Result{}, err
	}
	app.logger.Info(
		"pull request review complete",
		"pr", pull.Number,
		"head", pull.HeadSHA,
		"findings", len(findings),
		"published", published,
	)
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
