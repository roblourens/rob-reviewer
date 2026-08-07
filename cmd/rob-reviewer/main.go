package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/roblourens/rob-reviewer/internal/app"
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
		return errors.New("usage: rob-reviewer <poll|review> [options]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "poll":
		flags := flag.NewFlagSet("poll", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		stateRepository := flags.String("state-repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name repository used for durable state")
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
		result, err := reviewer.Poll(ctx, *stateRepository)
		if err != nil {
			return err
		}
		logger.Info(
			"poll complete",
			"bootstrapped", result.Bootstrapped,
			"reviewed", result.Reviewed,
			"deferred", result.Deferred,
			"skipped", result.Skipped,
		)
		return nil

	case "review":
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		configPath := flags.String("config", "reviewer.yaml", "path to reviewer configuration")
		pullRequest := flags.String("pr", "", "pull request number")
		publish := flags.Bool("publish", false, "publish the review instead of running safely in dry-run mode")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		number, err := app.ParsePullRequestNumber(*pullRequest)
		if err != nil {
			return err
		}
		reviewer, err := newApp(*configPath, logger)
		if err != nil {
			return err
		}
		defer func() {
			returnErr = errors.Join(returnErr, reviewer.Close())
		}()
		result, err := reviewer.ReviewPullRequest(ctx, number, *publish)
		if err != nil {
			return err
		}
		if err := app.PrintResult(result); err != nil {
			return err
		}
		return nil

	default:
		return fmt.Errorf("unknown command %q; use poll or review", os.Args[1])
	}
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
