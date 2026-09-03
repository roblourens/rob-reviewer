package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
)

const runRecordVersion = 1

type PollRunMetadata struct {
	RunID       string    `json:"runId"`
	RunAttempt  int       `json:"runAttempt"`
	EventName   string    `json:"eventName,omitempty"`
	Workflow    string    `json:"workflow,omitempty"`
	Repository  string    `json:"repository,omitempty"`
	CommitSHA   string    `json:"commitSha,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

type PollRunRecord struct {
	Version      int               `json:"version"`
	Metadata     PollRunMetadata   `json:"metadata"`
	Succeeded    bool              `json:"succeeded"`
	Error        string            `json:"error,omitempty"`
	Bootstrapped int               `json:"bootstrapped"`
	Reviewed     []int             `json:"reviewed"`
	Published    []int             `json:"published"`
	Deferred     []int             `json:"deferred"`
	Skipped      []int             `json:"skipped"`
	Reviews      []RunReviewRecord `json:"reviews"`
}

type RunReviewRecord struct {
	Number            int                `json:"number"`
	Title             string             `json:"title"`
	URL               string             `json:"url"`
	BaseSHA           string             `json:"baseSha"`
	HeadSHA           string             `json:"headSha"`
	FindingCount      int                `json:"findingCount"`
	Published         bool               `json:"published"`
	JSONReport        string             `json:"jsonReport"`
	MarkdownReport    string             `json:"markdownReport"`
	PublishedComments []RunCommentRecord `json:"publishedComments,omitempty"`
}

type RunCommentRecord struct {
	Path string      `json:"path"`
	Line int         `json:"line"`
	Side review.Side `json:"side"`
	Body string      `json:"body"`
}

func PollRunMetadataFromEnvironment(startedAt, completedAt time.Time, getenv func(string) string) PollRunMetadata {
	attempt, _ := strconv.Atoi(strings.TrimSpace(getenv("GITHUB_RUN_ATTEMPT")))
	return PollRunMetadata{
		RunID:       strings.TrimSpace(getenv("GITHUB_RUN_ID")),
		RunAttempt:  attempt,
		EventName:   strings.TrimSpace(getenv("GITHUB_EVENT_NAME")),
		Workflow:    strings.TrimSpace(getenv("GITHUB_WORKFLOW")),
		Repository:  strings.TrimSpace(getenv("GITHUB_REPOSITORY")),
		CommitSHA:   strings.TrimSpace(getenv("GITHUB_SHA")),
		StartedAt:   startedAt.UTC(),
		CompletedAt: completedAt.UTC(),
	}
}

func WritePollRunFiles(
	directory string,
	result PollResult,
	pollErr error,
	metadata PollRunMetadata,
) ([]string, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create poll run output directory: %w", err)
	}
	published := make(map[int]struct{}, len(result.Poll.Published))
	for _, number := range result.Poll.Published {
		published[number] = struct{}{}
	}
	reviews := make([]RunReviewRecord, 0, len(result.Reviews))
	for _, reviewResult := range result.Reviews {
		stem := fmt.Sprintf("pr-%d-%s", reviewResult.PullRequest.Number, shortSHA(reviewResult.PullRequest.HeadSHA))
		_, wasPublished := published[reviewResult.PullRequest.Number]
		record := RunReviewRecord{
			Number:         reviewResult.PullRequest.Number,
			Title:          reviewResult.PullRequest.Title,
			URL:            reviewResult.PullRequest.URL,
			BaseSHA:        reviewResult.PullRequest.BaseSHA,
			HeadSHA:        reviewResult.PullRequest.HeadSHA,
			FindingCount:   len(reviewResult.Findings),
			Published:      wasPublished,
			JSONReport:     stem + ".json",
			MarkdownReport: stem + ".md",
		}
		if wasPublished {
			for _, comment := range review.PublicComments(reviewResult) {
				record.PublishedComments = append(record.PublishedComments, RunCommentRecord(comment))
			}
		}
		reviews = append(reviews, record)
	}
	record := PollRunRecord{
		Version:      runRecordVersion,
		Metadata:     metadata,
		Succeeded:    pollErr == nil,
		Bootstrapped: result.Poll.Bootstrapped,
		Reviewed:     slices.Clone(result.Poll.Reviewed),
		Published:    slices.Clone(result.Poll.Published),
		Deferred:     slices.Clone(result.Poll.Deferred),
		Skipped:      slices.Clone(result.Poll.Skipped),
		Reviews:      reviews,
	}
	if pollErr != nil {
		record.Error = pollErr.Error()
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode poll run record: %w", err)
	}
	content = append(content, '\n')
	jsonPath := filepath.Join(directory, "run.json")
	markdownPath := filepath.Join(directory, "run.md")
	if err := writeFileAtomic(jsonPath, content); err != nil {
		return nil, fmt.Errorf("write JSON poll run record: %w", err)
	}
	if err := writeFileAtomic(markdownPath, []byte(formatPollRunMarkdown(record))); err != nil {
		return nil, fmt.Errorf("write Markdown poll run record: %w", err)
	}
	return []string{jsonPath, markdownPath}, nil
}

func formatPollRunMarkdown(record PollRunRecord) string {
	var output strings.Builder
	title := record.Metadata.RunID
	if title == "" {
		title = record.Metadata.StartedAt.Format(time.RFC3339)
	}
	fmt.Fprintf(&output, "# Reviewer run %s\n\n", review.SanitizeMarkdownText(title))
	fmt.Fprintf(&output, "- Status: **%s**\n", map[bool]string{true: "succeeded", false: "failed"}[record.Succeeded])
	fmt.Fprintf(&output, "- Started: `%s`\n", record.Metadata.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&output, "- Completed: `%s`\n", record.Metadata.CompletedAt.Format(time.RFC3339))
	if record.Metadata.CommitSHA != "" {
		fmt.Fprintf(&output, "- Reviewer commit: `%s`\n", review.SanitizeMarkdownText(record.Metadata.CommitSHA))
	}
	fmt.Fprintf(&output, "- Bootstrapped: %d\n", record.Bootstrapped)
	fmt.Fprintf(&output, "- Reviewed: %d\n", len(record.Reviewed))
	fmt.Fprintf(&output, "- Published: %d\n", len(record.Published))
	fmt.Fprintf(&output, "- Deferred: %d\n", len(record.Deferred))
	fmt.Fprintf(&output, "- Skipped: %d\n", len(record.Skipped))
	if record.Error != "" {
		fmt.Fprintf(&output, "\n## Error\n\n%s\n", review.SanitizeMarkdownTextWithCodeSpans(record.Error))
	}
	if len(record.Reviews) > 0 {
		output.WriteString("\n## Reviewed PRs\n")
	}
	for _, reviewed := range record.Reviews {
		fmt.Fprintf(
			&output,
			"\n### [#%d — %s](%s)\n\n- Head: `%s`\n- Findings: %d\n- Published: %t\n- Reports: [%s](%s), [%s](%s)\n",
			reviewed.Number,
			review.SanitizeMarkdownText(reviewed.Title),
			reviewed.URL,
			reviewed.HeadSHA,
			reviewed.FindingCount,
			reviewed.Published,
			reviewed.JSONReport,
			reviewed.JSONReport,
			reviewed.MarkdownReport,
			reviewed.MarkdownReport,
		)
		for _, comment := range reviewed.PublishedComments {
			fmt.Fprintf(
				&output,
				"\n#### Published comment at `%s:%d`\n\n%s\n",
				review.SanitizeMarkdownText(comment.Path),
				comment.Line,
				comment.Body,
			)
		}
	}
	if len(record.Reviewed) == 0 && len(record.Deferred) == 0 && len(record.Skipped) == 0 {
		output.WriteString("\nNo PR heads were processed by this run.\n")
	}
	return output.String()
}
