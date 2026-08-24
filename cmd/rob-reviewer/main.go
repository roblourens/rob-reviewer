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

	"github.com/roblourens/rob-reviewer/internal/app"
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
		return errors.New("usage: rob-reviewer <poll|review|publish-report> [options]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "poll":
		flags := flag.NewFlagSet("poll", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		stateRepository := flags.String("state-repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name repository used for durable state")
		outputDirectory := flags.String("output-dir", "", "directory for JSON and Markdown dry-run reports")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			return err
		}
		defer func() {
			returnErr = errors.Join(returnErr, reviewer.Close())
		}()
		result, err := reviewer.Poll(ctx, *stateRepository, *outputDirectory)
		if err != nil {
			return err
		}
		logger.Info(
			"poll complete",
			"bootstrapped", result.Poll.Bootstrapped,
			"reviewed", result.Poll.Reviewed,
			"deferred", result.Poll.Deferred,
			"skipped", result.Poll.Skipped,
			"reports", len(result.Reviews),
		)
		return nil

	case "review":
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		pullRequest := flags.String("pr", "", "pull request number")
		historical := flags.Bool("historical", false, "review a closed/merged PR without any publication or marker lookup")
		outputFormat := flags.String("format", "json", "local output format: json or markdown")
		outputDirectory := flags.String("output-dir", "", "directory for both JSON and Markdown dry-run reports")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		number, err := app.ParsePullRequestNumber(*pullRequest)
		if err != nil {
			return err
		}
		if *outputFormat != "json" && *outputFormat != "markdown" {
			return fmt.Errorf("--format must be json or markdown, got %q", *outputFormat)
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			return err
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
			return err
		}
		if !analyzed {
			logger.Info("pull request already has a review marker; no report was written", "pr", number)
			return nil
		}
		if *outputDirectory != "" {
			_, err := app.WriteResultFiles(*outputDirectory, result)
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

	default:
		return fmt.Errorf("unknown command %q; use poll, review, or publish-report", os.Args[1])
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
