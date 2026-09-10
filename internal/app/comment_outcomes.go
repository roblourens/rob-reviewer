package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/outcomes"
	"github.com/roblourens/rob-reviewer/internal/review"
)

const maxOutcomeLedgerBytes = 64 << 20

type OutcomePaths struct {
	JSON     string
	Markdown string
	RunsRoot string
}

func (app *App) TrackCommentOutcomes(ctx context.Context, paths OutcomePaths, numbers []int, maxPulls, maxAssessments int) error {
	return trackCommentOutcomes(ctx, paths, numbers, app.reviewClient, app, outcomes.Options{
		Owner: app.config.Target.Owner, Repo: app.config.Target.Repo,
		Model: app.config.Review.Model, ReasoningEffort: app.config.Review.ReasoningEffort,
		MaxPulls: maxPulls, MaxAssessments: maxAssessments,
	}, time.Now())
}

func (app *App) Assess(ctx context.Context, input outcomes.Input) (outcomes.Assessment, error) {
	runner, err := app.getRunner(ctx)
	if err != nil {
		return outcomes.Assessment{}, err
	}
	return runner.Assess(ctx, input)
}

func trackCommentOutcomes(ctx context.Context, paths OutcomePaths, numbers []int, reader outcomes.Reader, classifier outcomes.Classifier, options outcomes.Options, now time.Time) error {
	if strings.TrimSpace(paths.JSON) == "" || strings.TrimSpace(paths.Markdown) == "" {
		return errors.New("outcome JSON and Markdown paths are required")
	}
	jsonPath, err := filepath.Abs(paths.JSON)
	if err != nil {
		return err
	}
	markdownPath, err := filepath.Abs(paths.Markdown)
	if err != nil {
		return err
	}
	if jsonPath == markdownPath {
		return errors.New("outcome JSON and Markdown paths must be different")
	}
	ledger := outcomes.Ledger{
		Version: outcomes.Version, Repository: options.Owner + "/" + options.Repo,
		Pulls: []outcomes.PullRecord{},
	}
	if _, err := os.Stat(paths.JSON); err == nil {
		if err := decodeStrictJSONFile(paths.JSON, &ledger, maxOutcomeLedgerBytes); err != nil {
			return fmt.Errorf("load comment outcome ledger: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat comment outcome ledger: %w", err)
	}
	if ledger.Version != outcomes.Version || !strings.EqualFold(ledger.Repository, options.Owner+"/"+options.Repo) {
		return errors.New("outcome ledger version or repository does not match")
	}
	if paths.RunsRoot != "" {
		archivedNumbers, err := publishedPullNumbers(paths.RunsRoot)
		if err != nil {
			return err
		}
		numbers = append(slices.Clone(numbers), archivedNumbers...)
	}
	if options.MaxPulls <= 0 || options.MaxAssessments <= 0 {
		return errors.New("outcome refresh limits must be positive")
	}
	for _, number := range numbers {
		if number <= 0 {
			return errors.New("outcome pull request numbers must be positive")
		}
	}
	refreshErr := outcomes.Refresh(ctx, &ledger, numbers, reader, classifier, options, now)
	return errors.Join(refreshErr, writeCommentOutcomes(paths, ledger))
}

func publishedPullNumbers(root string) ([]int, error) {
	paths, err := archivedRunPaths(root)
	if err != nil {
		return nil, err
	}
	var numbers []int
	for _, path := range paths {
		record, err := loadPollRunRecord(path)
		if err != nil {
			return nil, err
		}
		for _, reviewed := range record.Reviews {
			if reviewed.Published {
				numbers = append(numbers, reviewed.Number)
			}
		}
	}
	slices.Sort(numbers)
	return slices.Compact(numbers), nil
}

func writeCommentOutcomes(paths OutcomePaths, ledger outcomes.Ledger) error {
	content, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode comment outcome ledger: %w", err)
	}
	content = append(content, '\n')
	if len(content) > maxOutcomeLedgerBytes {
		return errors.New("comment outcome ledger exceeds 64 MiB")
	}
	for _, path := range []string{paths.JSON, paths.Markdown} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create comment outcome directory: %w", err)
		}
	}
	if err := writeFileAtomic(paths.JSON, content); err != nil {
		return fmt.Errorf("write comment outcome ledger: %w", err)
	}
	if err := writeFileAtomic(paths.Markdown, []byte(formatCommentOutcomes(ledger))); err != nil {
		return fmt.Errorf("write comment outcome statistics: %w", err)
	}
	return nil
}

func formatCommentOutcomes(ledger outcomes.Ledger) string {
	var output strings.Builder
	stats := outcomes.Summarize(ledger)
	fmt.Fprintf(&output, "# Review comment outcomes\n\nRepository: `%s`. Updated: %s.\n\n", ledger.Repository, ledger.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(&output, "Tracked comments: **%d**. PR refresh errors: **%d**.\n\n", stats.Total, stats.FailedPulls)
	output.WriteString("Counts describe the last successful observation for each PR, not live GitHub state. See each PR's checked timestamp below. Unavailable includes missing threads and failed refreshes.\n\n")
	output.WriteString("Resolved does not mean fixed or accepted. Ignored means closed/merged, unresolved, and no human reply other than the reviewer; it is a proxy, not proof of intent. Open silent threads remain pending. Outdated is not a resolution.\n\n")
	writeCounts := func(title string, keys []string, counts map[string]int) {
		fmt.Fprintf(&output, "## %s\n\n| Category | Comments | %% of tracked comments |\n| --- | ---: | ---: |\n", title)
		for _, key := range keys {
			percentage := 0.0
			if stats.Total > 0 {
				percentage = 100 * float64(counts[key]) / float64(stats.Total)
			}
			fmt.Fprintf(&output, "| %s | %d | %.1f%% |\n", key, counts[key], percentage)
		}
		output.WriteString("\n")
	}
	writeCounts("Thread outcome", []string{"resolved", "replied", "ignored", "pending", "unavailable"}, stats.Outcomes)
	fmt.Fprintf(&output, "Author feedback is independent of resolution. AI assessments below %.0f%% confidence count as unclear. Mixed preserves disagreement even after later acceptance. Accepted means expressed agreement, not a verified code fix.\n\n", 100*outcomes.ConfidenceThreshold)
	writeCounts("PR-author feedback", []string{"accepted", "disagreed", "mixed", "unclear", "no-author-reply", "awaiting-assessment", "error", "unavailable"}, stats.AuthorFeedback)
	for _, pull := range ledger.Pulls {
		fmt.Fprintf(&output, "## PR %d\n\n", pull.Number)
		if pull.URL != "" {
			fmt.Fprintf(&output, "[%s#%d](%s)\n\n", ledger.Repository, pull.Number, pull.URL)
		}
		fmt.Fprintf(&output, "State: %s. Checked: %s.\n\n", pull.State, pull.LastCheckedAt.Format(time.RFC3339))
		if pull.Error != "" {
			fmt.Fprintf(&output, "Refresh error: %s\n\n", review.SanitizeMarkdownText(pull.Error))
		}
		for _, record := range pull.Comments {
			if len(record.Thread.Comments) == 0 {
				continue
			}
			root := record.Thread.Comments[0]
			fmt.Fprintf(&output, "- [Comment %d](%s) at `%s`: **%s**; author feedback: **%s**; outdated: %t.\n",
				root.ID, root.URL, review.SanitizeMarkdownText(root.Path), outcomes.Outcome(pull, record), outcomes.Feedback(pull, record), record.Thread.Outdated)
			if record.Assessment != nil {
				assessment := record.Assessment
				fmt.Fprintf(&output, "  - AI assessment (last observed): %s, %.0f%% confidence (%s). %s\n",
					assessment.Feedback, 100*assessment.Confidence, review.SanitizeMarkdownText(record.AssessmentModel), review.SanitizeMarkdownText(assessment.Rationale))
				for _, evidence := range assessment.Evidence {
					fmt.Fprintf(&output, "  - Evidence, comment %d: %s\n", evidence.CommentID, review.SanitizeMarkdownText(evidence.Quote))
				}
			}
			if record.AssessmentError != "" {
				fmt.Fprintf(&output, "  - Assessment error: %s\n", review.SanitizeMarkdownText(record.AssessmentError))
			}
		}
		output.WriteString("\n")
	}
	output.WriteString("(Written by Copilot)\n")
	return output.String()
}
