package review

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
				"**[%s] %s**\n\n%s\n\n**Evidence:** %s\n\n**Suggested direction:** %s\n\n%s",
				finding.Severity,
				sanitizeMarkdownText(finding.Title),
				sanitizeMarkdownText(finding.Impact),
				sanitizeMarkdownText(finding.Evidence),
				sanitizeMarkdownText(finding.Recommendation),
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
	fmt.Fprintf(&summary, "\n%s\n\n%s", generatedDisclosure, marker)

	return ReviewRequest{
		CommitID: result.PullRequest.HeadSHA,
		Event:    "COMMENT",
		Body:     summary.String(),
		Comments: comments,
	}
}

func validatePublicationTarget(analyzed, current PullRequest) error {
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

func sanitizeMarkdownText(value string) string {
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
