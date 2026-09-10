package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/roblourens/rob-reviewer/internal/app"
	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/poller"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("rob-reviewer failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) (returnErr error) {
	if len(os.Args) < 2 {
		return errors.New("usage: rob-reviewer <poll|review|publish-report|apply-learnings|update-published-index|track-comment-outcomes> [options]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "poll":
		startedAt := time.Now()
		flags := flag.NewFlagSet("poll", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		stateRepository := flags.String("state-repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name repository used for durable state")
		outputDirectory := flags.String("output-dir", "", "directory for JSON and Markdown dry-run reports")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			_, recordErr := app.WritePollRunFiles(
				*outputDirectory,
				app.PollResult{},
				err,
				app.PollRunMetadataFromEnvironment(startedAt, time.Now(), os.Getenv),
			)
			return errors.Join(err, recordErr)
		}
		defer func() {
			returnErr = errors.Join(returnErr, reviewer.Close())
		}()
		result, pollErr := reviewer.Poll(ctx, *stateRepository, *outputDirectory)
		_, recordErr := app.WritePollRunFiles(
			*outputDirectory,
			result,
			pollErr,
			app.PollRunMetadataFromEnvironment(startedAt, time.Now(), os.Getenv),
		)
		if pollErr != nil || recordErr != nil {
			return errors.Join(pollErr, recordErr)
		}
		logger.Info(
			"poll complete",
			"bootstrapped", result.Poll.Bootstrapped,
			"reviewed", result.Poll.Reviewed,
			"published", result.Poll.Published,
			"deferred", result.Poll.Deferred,
			"skipped", result.Poll.Skipped,
			"reports", len(result.Reviews),
		)
		return nil

	case "review":
		startedAt := time.Now()
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		pullRequest := flags.String("pr", "", "pull request number")
		historical := flags.Bool("historical", false, "review a closed/merged PR without any publication or marker lookup")
		outputFormat := flags.String("format", "json", "local output format: json or markdown")
		outputDirectory := flags.String("output-dir", "", "directory for both JSON and Markdown dry-run reports")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		recordFailure := func(runErr error) error {
			if strings.TrimSpace(*outputDirectory) == "" {
				return runErr
			}
			_, recordErr := app.WritePollRunFiles(
				*outputDirectory,
				app.PollResult{},
				runErr,
				app.PollRunMetadataFromEnvironment(startedAt, time.Now(), os.Getenv),
			)
			return errors.Join(runErr, recordErr)
		}
		number, err := app.ParsePullRequestNumber(*pullRequest)
		if err != nil {
			return recordFailure(err)
		}
		if *outputFormat != "json" && *outputFormat != "markdown" {
			return recordFailure(fmt.Errorf("--format must be json or markdown, got %q", *outputFormat))
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			return recordFailure(err)
		}
		defer func() {
			returnErr = errors.Join(returnErr, reviewer.Close())
		}()
		var result review.Result
		analyzed := true
		if *historical {
			result, err = reviewer.ReviewHistoricalPullRequest(ctx, number)
		} else {
			result, analyzed, err = reviewer.ReviewPullRequest(ctx, number, false)
		}
		if err != nil {
			return recordFailure(err)
		}
		if !analyzed {
			logger.Info("pull request already has a review marker; no report was written", "pr", number)
			if strings.TrimSpace(*outputDirectory) == "" {
				return nil
			}
			_, err := app.WritePollRunFiles(
				*outputDirectory,
				app.PollResult{Poll: poller.Result{Skipped: []int{number}}},
				nil,
				app.PollRunMetadataFromEnvironment(startedAt, time.Now(), os.Getenv),
			)
			return err
		}
		if *outputDirectory != "" {
			if _, err := app.WriteResultFiles(*outputDirectory, result); err != nil {
				return recordFailure(err)
			}
			_, err := app.WritePollRunFiles(
				*outputDirectory,
				app.PollResult{
					Poll:    poller.Result{Reviewed: []int{result.PullRequest.Number}},
					Reviews: []review.Result{result},
				},
				nil,
				app.PollRunMetadataFromEnvironment(startedAt, time.Now(), os.Getenv),
			)
			return err
		}
		var printErr error
		if *outputFormat == "markdown" {
			printErr = app.PrintResultMarkdown(result)
		} else {
			printErr = app.PrintResult(result)
		}
		if printErr != nil {
			return printErr
		}
		return nil

	case "publish-report":
		flags := flag.NewFlagSet("publish-report", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		reportPath := flags.String("report", "", "saved JSON dry-run report")
		var approvedIDs stringListFlag
		flags.Var(&approvedIDs, "finding", "approved finding ID; repeat for multiple findings")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if strings.TrimSpace(*reportPath) == "" {
			return errors.New("--report is required")
		}
		publisher, err := app.NewSavedReportPublisher(*configPath, os.Getenv("REVIEW_GITHUB_TOKEN"))
		if err != nil {
			return err
		}
		selected, published, err := publisher.Publish(ctx, *reportPath, approvedIDs)
		if err != nil {
			return err
		}
		logger.Info(
			"approved performance review publication complete",
			"pr", selected.PullRequest.Number,
			"head", selected.PullRequest.HeadSHA,
			"findingIDs", approvedIDs,
			"published", published,
		)
		return nil

	case "apply-learnings":
		flags := flag.NewFlagSet("apply-learnings", flag.ContinueOnError)
		repository := flags.String("repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name repository containing reviewer guidance")
		branch := flags.String("branch", "main", "branch containing reviewer guidance")
		proposalsPath := flags.String("proposals-path", "", "learning-proposals.json file or directory tree containing proposal files")
		casesPath := flags.String("cases-path", "", "current-run regression-cases.json file authorizing the proposals")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if strings.TrimSpace(*proposalsPath) == "" {
			return errors.New("--proposals-path is required")
		}
		if strings.TrimSpace(*casesPath) == "" {
			return errors.New("--cases-path is required")
		}
		changed, err := app.PublishLearnedGuidance(
			ctx,
			*repository,
			*branch,
			*proposalsPath,
			*casesPath,
			os.Getenv("GITHUB_TOKEN"),
		)
		if err != nil {
			return err
		}
		logger.Info("learned performance guidance processed", "changed", changed)
		return nil

	case "update-published-index":
		flags := flag.NewFlagSet("update-published-index", flag.ContinueOnError)
		indexPath := flags.String("index", "", "published review JSON index to create or update")
		markdownPath := flags.String("markdown", "", "published review Markdown index to create or update")
		runPath := flags.String("run", "", "single run.json to merge")
		runsRoot := flags.String("runs-root", "", "run archive tree to reconcile")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		for name, value := range map[string]string{
			"--index":    *indexPath,
			"--markdown": *markdownPath,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required", name)
			}
		}
		if strings.TrimSpace(*runPath) == "" && strings.TrimSpace(*runsRoot) == "" {
			return errors.New("either --run or --runs-root is required")
		}
		if strings.TrimSpace(*runPath) != "" && strings.TrimSpace(*runsRoot) != "" {
			return errors.New("--run and --runs-root are mutually exclusive")
		}
		if strings.TrimSpace(*runsRoot) != "" {
			return app.WritePublishedReviewIndexFromArchive(*indexPath, *markdownPath, *runsRoot)
		}
		return app.WritePublishedReviewIndex(*indexPath, *markdownPath, *runPath)

	case "track-comment-outcomes":
		flags := flag.NewFlagSet("track-comment-outcomes", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		jsonPath := flags.String("ledger", "", "durable comment outcome JSON ledger")
		markdownPath := flags.String("markdown", "", "generated Markdown statistics")
		runsRoot := flags.String("runs-root", "", "full archived run tree for publication discovery")
		maxPulls := flags.Int("max-prs", 100, "maximum PRs to refresh, least recently attempted first")
		maxAssessments := flags.Int("max-assessments", 10, "maximum changed conversations to classify")
		automatic := flags.Bool("automatic", false, "respect automation.enabled for scheduled operation")
		var pullRequests stringListFlag
		flags.Var(&pullRequests, "pr", "additional published PR to track; repeat for multiple PRs")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *automatic {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			if !cfg.Automation.Enabled {
				logger.Info("automatic comment tracking is disabled by configuration")
				return nil
			}
		}
		var numbers []int
		for _, value := range pullRequests {
			number, err := app.ParsePullRequestNumber(value)
			if err != nil {
				return err
			}
			numbers = append(numbers, number)
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			return err
		}
		defer func() {
			returnErr = errors.Join(returnErr, reviewer.Close())
		}()
		return reviewer.TrackCommentOutcomes(ctx, app.OutcomePaths{
			JSON: *jsonPath, Markdown: *markdownPath, RunsRoot: *runsRoot,
		}, numbers, *maxPulls, *maxAssessments)

	default:
		return fmt.Errorf("unknown command %q; use poll, review, publish-report, apply-learnings, update-published-index, or track-comment-outcomes", os.Args[1])
	}
}

type stringListFlag []string

func (flag *stringListFlag) String() string {
	return strings.Join(*flag, ",")
}

func (flag *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("finding ID cannot be empty")
	}
	*flag = append(*flag, value)
	return nil
}

func newApp(configPath string, logger *slog.Logger) (*app.App, error) {
	return app.New(app.Options{
		ConfigPath:   configPath,
		Logger:       logger,
		StateToken:   os.Getenv("GITHUB_TOKEN"),
		ReviewToken:  os.Getenv("REVIEW_GITHUB_TOKEN"),
		CopilotToken: os.Getenv("COPILOT_GITHUB_TOKEN"),
	})
}
