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
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
)

func (publisher *SavedReportPublisher) PublishRun(ctx context.Context, directory string, expected PollRunMetadata, logger *slog.Logger) (returnErr error) {
	if !publisher.config.Automation.Enabled || publisher.config.Publication.Mode != "automatic" {
		logger.Info("automatic publication is disabled")
		return nil
	}
	if publisher.config.Publication.OptIn == nil || publisher.check == nil {
		return errors.New("automatic run publication requires an author opt-in file")
	}
	record, err := loadPollRunRecord(filepath.Join(directory, "run.json"))
	if err != nil {
		return err
	}
	if expected.RunID == "" || expected.RunAttempt < 1 || expected.Repository == "" || expected.CommitSHA == "" ||
		record.Metadata.RunID != expected.RunID || record.Metadata.RunAttempt != expected.RunAttempt ||
		record.Metadata.Repository != expected.Repository || record.Metadata.CommitSHA != expected.CommitSHA {
		return errors.New("review artifact does not match the expected workflow run, attempt, repository and commit")
	}
	if len(record.Reviews) > publisher.config.Poll.MaxPerRun {
		return errors.New("review artifact exceeds the configured per-run review limit")
	}

	results := make(map[int]review.Result)
	for _, entry := range record.Reviews {
		if !slices.Contains(record.Reviewed, entry.Number) {
			continue
		}
		if _, duplicate := results[entry.Number]; duplicate {
			return errors.New("review artifact contains duplicate PRs")
		}
		expectedURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", publisher.config.Target.Owner, publisher.config.Target.Repo, entry.Number)
		expectedReport := fmt.Sprintf("pr-%d-%s.json", entry.Number, shortSHA(entry.HeadSHA))
		if entry.URL != expectedURL || entry.JSONReport != expectedReport {
			return errors.New("review artifact contains a different publication target or unexpected report path")
		}
		path := filepath.Join(directory, entry.JSONReport)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect publication report: %w", err)
		}
		if !info.Mode().IsRegular() {
			return errors.New("publication report must be a regular file")
		}
		result, err := LoadResult(path)
		if err != nil {
			return err
		}
		if result.PullRequest.Number != entry.Number || result.PullRequest.URL != expectedURL ||
			result.PullRequest.BaseSHA != entry.BaseSHA || result.PullRequest.HeadSHA != entry.HeadSHA ||
			len(result.Findings) != entry.FindingCount {
			return errors.New("publication report does not match its run manifest")
		}
		results[entry.Number] = result
	}

	oldCommentCount := 0
	for _, entry := range record.Reviews {
		oldCommentCount += len(entry.PublishedComments)
	}
	defer func() {
		record.PublicationError = ""
		if returnErr != nil {
			record.PublicationError = returnErr.Error()
		}
		record.Succeeded = record.Error == "" && returnErr == nil
		content, err := json.MarshalIndent(record, "", "  ")
		if err == nil {
			err = writeFileAtomic(filepath.Join(directory, "run.json"), append(content, '\n'))
		}
		if err == nil {
			err = writeFileAtomic(filepath.Join(directory, "run.md"), []byte(formatPollRunMarkdown(record)))
		}
		if err == nil {
			_, err = WritePublishedCommentsFile(directory, record)
		}
		if err == nil {
			newCommentCount := 0
			for _, entry := range record.Reviews {
				newCommentCount += len(entry.PublishedComments)
			}
			if newCommentCount != oldCommentCount {
				err = os.Remove(filepath.Join(directory, fmt.Sprintf("published-comments-%d.md", oldCommentCount)))
				if errors.Is(err, os.ErrNotExist) {
					err = nil
				}
			}
		}
		returnErr = errors.Join(returnErr, err)
	}()

	automatic := review.NewAutomaticPublisher(
		publisher.client, publisher.config.Target.Owner, publisher.config.Target.Repo,
		publisher.config.Automation.SkipLabels...,
	).WithPublicationCheck(publisher.check)
	for index := range record.Reviews {
		entry := &record.Reviews[index]
		result, exists := results[entry.Number]
		if !exists || entry.Published || len(result.Findings) == 0 {
			continue
		}
		current, err := publisher.client.GetPullRequest(ctx, publisher.config.Target.Owner, publisher.config.Target.Repo, entry.Number)
		if err != nil {
			return fmt.Errorf("refresh PR %d for automatic publication: %w", entry.Number, err)
		}
		if err := publisher.check(ctx, current); err != nil {
			if errors.Is(err, review.ErrPublicationSuppressed) {
				logger.Info("automatic publication suppressed", "pr", entry.Number, "reason", err)
				continue
			}
			return err
		}
		if !strings.EqualFold(current.State, "open") || current.HeadSHA != result.PullRequest.HeadSHA ||
			current.BaseSHA != result.PullRequest.BaseSHA || !current.IsTeamAuthored() ||
			current.HasAnyLabel(publisher.config.Automation.SkipLabels) || current.Draft && !result.PullRequest.Draft {
			logger.Info("automatic publication skipped because the PR is no longer eligible or its revision changed", "pr", entry.Number)
			continue
		}
		existing, err := automatic.PrepareAutomaticReview(ctx, result.PullRequest)
		if errors.Is(err, review.ErrPublicationSuppressed) {
			logger.Info("automatic publication suppressed", "pr", entry.Number, "reason", err)
			continue
		}
		if err != nil {
			return err
		}
		var comments []review.Comment
		if existing != nil {
			if !strings.EqualFold(existing.State, "PENDING") {
				continue
			}
			comments, err = automatic.ResumePending(ctx, result.PullRequest, *existing)
		} else {
			var published bool
			published, err = automatic.Publish(ctx, result, false)
			if published {
				comments = review.PublicComments(result)
			}
		}
		if errors.Is(err, review.ErrPublicationSuppressed) {
			logger.Info("automatic publication suppressed", "pr", entry.Number, "reason", err)
			continue
		}
		if err != nil {
			return fmt.Errorf("publish PR %d: %w", entry.Number, err)
		}
		if len(comments) == 0 {
			continue
		}
		entry.Published = true
		for _, comment := range comments {
			entry.PublishedComments = append(entry.PublishedComments, RunCommentRecord(comment))
		}
		if !slices.Contains(record.Published, entry.Number) {
			record.Published = append(record.Published, entry.Number)
		}
		logger.Info("automatic review published", "pr", entry.Number, "comments", len(comments))
	}
	return nil
}
