package review

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const generatedDisclosure = "(Written by Copilot)"

type ReviewRequest struct {
	CommitID string
	Event    string
	Body     string
	Comments []Comment
}

type Comment struct {
	Path string
	Line int
	Side Side
	Body string
}

type PublisherClient interface {
	GetPullRequest(ctx context.Context, owner, repo string, number int) (PullRequest, error)
	HasReviewMarker(ctx context.Context, owner, repo string, number int, marker string) (bool, error)
	PublishReview(ctx context.Context, owner, repo string, number int, request ReviewRequest) error
}

type Publisher struct {
	client PublisherClient
	owner  string
	repo   string
}

func NewPublisher(client PublisherClient, owner, repo string) *Publisher {
	return &Publisher{client: client, owner: owner, repo: repo}
}

func (publisher *Publisher) Publish(ctx context.Context, result Result, dryRun bool) (bool, error) {
	current, err := publisher.client.GetPullRequest(ctx, publisher.owner, publisher.repo, result.PullRequest.Number)
	if err != nil {
		return false, fmt.Errorf("refresh pull request before publication: %w", err)
	}
	if err := validatePublicationTarget(result.PullRequest, current); err != nil {
		return false, err
	}
	if len(result.Findings) == 0 {
		return false, nil
	}

	marker := fmt.Sprintf("<!-- rob-reviewer:v1 pr=%d head=%s -->", result.PullRequest.Number, result.PullRequest.HeadSHA)
	exists, err := publisher.client.HasReviewMarker(ctx, publisher.owner, publisher.repo, result.PullRequest.Number, marker)
	if err != nil {
		return false, fmt.Errorf("check existing review marker: %w", err)
	}
	if exists {
		return false, nil
	}

	request := buildReviewRequest(result, marker)
	if dryRun {
		return false, nil
	}
	current, err = publisher.client.GetPullRequest(ctx, publisher.owner, publisher.repo, result.PullRequest.Number)
	if err != nil {
		return false, fmt.Errorf("refresh pull request immediately before publication: %w", err)
	}
	if err := validatePublicationTarget(result.PullRequest, current); err != nil {
		return false, err
	}
	if err := publisher.client.PublishReview(ctx, publisher.owner, publisher.repo, result.PullRequest.Number, request); err != nil {
		return false, err
	}
	return true, nil
}

func buildReviewRequest(result Result, marker string) ReviewRequest {
	comments := make([]Comment, 0, len(result.Findings))
	focusCounts := make(map[string]int)
	for _, finding := range result.Findings {
		focusCounts[finding.Focus]++
		comments = append(comments, Comment{
			Path: finding.Path,
			Line: finding.Line,
			Side: finding.Side,
			Body: fmt.Sprintf(
				"**[%s] %s**\n\n%s\n\n**Confidence:** %.2f — %s\n\n**Evidence:** %s\n\n**Suggested direction:** %s\n\n%s",
				finding.Severity,
				SanitizeMarkdownText(finding.Title),
				SanitizeMarkdownText(finding.Impact),
				finding.Confidence,
				SanitizeMarkdownText(finding.ConfidenceRationale),
				SanitizeMarkdownText(finding.Evidence),
				SanitizeMarkdownText(finding.Recommendation),
				generatedDisclosure,
			),
		})
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Found %d high-confidence review", len(result.Findings))
	if len(result.Findings) == 1 {
		summary.WriteString(" finding.")
	} else {
		summary.WriteString(" findings.")
	}
	summary.WriteString("\n\n")
	for _, focus := range sortedMapKeys(focusCounts) {
		fmt.Fprintf(&summary, "- `%s`: %d\n", focus, focusCounts[focus])
	}
	fmt.Fprintf(&summary, "\n%s\n\n%s\n\n%s", FormatStatsMarkdown(result.Stats), generatedDisclosure, marker)

	return ReviewRequest{
		CommitID: result.PullRequest.HeadSHA,
		Event:    "COMMENT",
		Body:     summary.String(),
		Comments: comments,
	}
}

func FormatStatsMarkdown(stats Stats) string {
	var output strings.Builder
	output.WriteString("<details>\n<summary>Review statistics</summary>\n\n")
	fmt.Fprintf(&output, "- Model: `%s`\n", stats.Model)
	if len(stats.ActualModels) > 0 {
		fmt.Fprintf(&output, "- Actual model calls: `%s`\n", strings.Join(stats.ActualModels, "`, `"))
	}
	fmt.Fprintf(&output, "- Reasoning effort: `%s`\n", stats.ReasoningEffort)
	if len(stats.ActualReasoningEfforts) > 0 {
		fmt.Fprintf(&output, "- Actual reasoning effort: `%s`\n", strings.Join(stats.ActualReasoningEfforts, "`, `"))
	}
	if len(stats.APIEndpoints) > 0 {
		fmt.Fprintf(&output, "- Model API endpoint: `%s`\n", strings.Join(stats.APIEndpoints, "`, `"))
	}
	fmt.Fprintf(&output, "- Wall-clock time: %s\n", formatDuration(stats.WallClockMilliseconds))
	fmt.Fprintf(&output, "- Model calls: %d\n", stats.ModelCalls)
	fmt.Fprintf(
		&output,
		"- Tokens: %d input, %d output, %d total; %d reasoning; %d cache read, %d cache write\n",
		stats.InputTokens,
		stats.OutputTokens,
		stats.TotalTokens,
		stats.ReasoningTokens,
		stats.CacheReadTokens,
		stats.CacheWriteTokens,
	)
	fmt.Fprintf(&output, "- Aggregate model API time: %s\n", formatDuration(stats.APIDurationMilliseconds))
	fmt.Fprintf(&output, "- Model-returned tool calls: %d\n", stats.ToolCalls)
	fmt.Fprintf(&output, "- Copilot usage: %.3f nano-AI units", stats.NanoAIUnits)
	if len(stats.ModelBillingMultipliers) > 0 {
		output.WriteString("; model billing multiplier")
		if len(stats.ModelBillingMultipliers) > 1 {
			output.WriteString("s")
		}
		output.WriteString(": ")
		for index, multiplier := range stats.ModelBillingMultipliers {
			if index > 0 {
				output.WriteString(", ")
			}
			fmt.Fprintf(&output, "%.3f", multiplier)
		}
	}
	if stats.PremiumRequests != nil {
		fmt.Fprintf(&output, "; %.3f premium requests", *stats.PremiumRequests)
	}
	output.WriteString("\n")
	output.WriteString("- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.\n")
	output.WriteString("\n</details>")
	return output.String()
}

func formatDuration(milliseconds int64) string {
	return (time.Duration(milliseconds) * time.Millisecond).Round(time.Millisecond).String()
}

func validatePublicationTarget(analyzed, current PullRequest) error {
	if !current.IsTeamAuthored() {
		return fmt.Errorf(
			"pull request %d is no longer team-authored (author %q, association %q)",
			analyzed.Number,
			current.AuthorLogin,
			current.AuthorAssociation,
		)
	}
	if !strings.EqualFold(current.State, "open") {
		return fmt.Errorf("pull request %d became %s during review", analyzed.Number, current.State)
	}
	if current.Draft {
		return fmt.Errorf("pull request %d became a draft during review", analyzed.Number)
	}
	if current.BaseSHA != analyzed.BaseSHA {
		return fmt.Errorf("pull request base changed from %s to %s during review", analyzed.BaseSHA, current.BaseSHA)
	}
	if current.HeadSHA != analyzed.HeadSHA {
		return fmt.Errorf("pull request head changed from %s to %s during review", analyzed.HeadSHA, current.HeadSHA)
	}
	return nil
}

func SanitizeMarkdownText(value string) string {
	var clean strings.Builder
	clean.Grow(len(value))
	for _, character := range value {
		if character == '\n' || character == '\t' || character >= ' ' {
			clean.WriteRune(character)
		}
	}
	return strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		"*", `\*`,
		"_", `\_`,
		"[", `\[`,
		"]", `\]`,
		"<", "&lt;",
		">", "&gt;",
		"#", `\#`,
		"!", `\!`,
		"|", `\|`,
		"@", "&#64;",
	).Replace(clean.String())
}

func sortedMapKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
