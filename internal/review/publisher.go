package review

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	generatedDisclosure      = "(Written by Copilot)"
	experimentalReviewPrefix = "**[Experimental performance review bot]**"
)

var ErrPublicationSuppressed = errors.New("review publication suppressed")

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

type ExistingReview struct {
	ID    int64
	State string
	Body  string
}

type PublisherClient interface {
	GetPullRequest(ctx context.Context, owner, repo string, number int) (PullRequest, error)
	FindReviewMarker(ctx context.Context, owner, repo string, number int, marker string) (*ExistingReview, error)
	FindPendingReview(ctx context.Context, owner, repo string, number int) (*ExistingReview, error)
	CreatePendingReview(ctx context.Context, owner, repo string, number int, request ReviewRequest) (int64, error)
	SubmitPendingReview(ctx context.Context, owner, repo string, number int, reviewID int64) error
	DeletePendingReview(ctx context.Context, owner, repo string, number int, reviewID int64) error
}

type Publisher struct {
	client      PublisherClient
	owner       string
	repo        string
	skipLabels  []string
	allowClosed bool
}

func NewPublisher(client PublisherClient, owner, repo string, skipLabels ...string) *Publisher {
	return &Publisher{
		client:      client,
		owner:       owner,
		repo:        repo,
		skipLabels:  slices.Clone(skipLabels),
		allowClosed: true,
	}
}

func NewAutomaticPublisher(client PublisherClient, owner, repo string, skipLabels ...string) *Publisher {
	return &Publisher{
		client:      client,
		owner:       owner,
		repo:        repo,
		skipLabels:  slices.Clone(skipLabels),
		allowClosed: false,
	}
}

func (publisher *Publisher) Publish(ctx context.Context, result Result, dryRun bool) (bool, error) {
	current, err := publisher.client.GetPullRequest(ctx, publisher.owner, publisher.repo, result.PullRequest.Number)
	if err != nil {
		return false, fmt.Errorf("refresh pull request before publication: %w", err)
	}

	if err := publisher.validatePublicationTarget(result.PullRequest, current); err != nil {
		return false, err
	}
	if len(result.Findings) == 0 {
		return false, nil
	}

	marker := fmt.Sprintf("<!-- rob-reviewer:v1 pr=%d head=%s -->", result.PullRequest.Number, result.PullRequest.HeadSHA)
	approvalMarker := publicationApprovalMarker(result)
	existing, err := publisher.client.FindReviewMarker(ctx, publisher.owner, publisher.repo, result.PullRequest.Number, marker)
	if err != nil {
		return false, fmt.Errorf("check existing review marker: %w", err)
	}
	if existing != nil && !strings.EqualFold(existing.State, "PENDING") {
		return false, nil
	}
	if existing != nil && !strings.Contains(existing.Body, approvalMarker) {
		return false, errors.New("a pending performance review exists with a different approved finding set")
	}

	if dryRun {
		return false, nil
	}
	current, err = publisher.client.GetPullRequest(ctx, publisher.owner, publisher.repo, result.PullRequest.Number)
	if err != nil {
		return false, fmt.Errorf("refresh pull request immediately before publication: %w", err)
	}
	if err := publisher.validatePublicationTarget(result.PullRequest, current); err != nil {
		return false, err
	}
	if existing != nil {
		if err := publisher.client.SubmitPendingReview(
			ctx,
			publisher.owner,
			publisher.repo,
			result.PullRequest.Number,
			existing.ID,
		); err != nil {
			return false, fmt.Errorf("resume pending performance review %d: %w", existing.ID, err)
		}
		return true, nil
	}
	request := buildReviewRequest(result, marker+"\n"+approvalMarker)
	pendingReviewID, err := publisher.client.CreatePendingReview(
		ctx,
		publisher.owner,
		publisher.repo,
		result.PullRequest.Number,
		request,
	)
	if err != nil {
		return false, fmt.Errorf("create pending performance review claim: %w", err)
	}
	if pendingReviewID == 0 {
		return false, errors.New("GitHub returned an invalid pending review ID")
	}
	if err := publisher.client.SubmitPendingReview(
		ctx,
		publisher.owner,
		publisher.repo,
		result.PullRequest.Number,
		pendingReviewID,
	); err != nil {
		return false, fmt.Errorf("submit pending performance review %d: %w", pendingReviewID, err)
	}
	return true, nil
}

func (publisher *Publisher) ResumePending(
	ctx context.Context,
	analyzed PullRequest,
	reviewID int64,
) error {
	current, err := publisher.client.GetPullRequest(ctx, publisher.owner, publisher.repo, analyzed.Number)
	if err != nil {
		return fmt.Errorf("refresh pull request before resuming pending review: %w", err)
	}
	if err := publisher.validatePublicationTarget(analyzed, current); err != nil {
		if errors.Is(err, ErrPublicationSuppressed) {
			if deleteErr := publisher.client.DeletePendingReview(
				ctx,
				publisher.owner,
				publisher.repo,
				analyzed.Number,
				reviewID,
			); deleteErr != nil {
				return errors.Join(err, fmt.Errorf("delete suppressed pending review %d: %w", reviewID, deleteErr))
			}
		}
		return err
	}
	if err := publisher.client.SubmitPendingReview(ctx, publisher.owner, publisher.repo, analyzed.Number, reviewID); err != nil {
		return fmt.Errorf("resume pending performance review %d: %w", reviewID, err)
	}
	return nil
}

func (publisher *Publisher) PrepareAutomaticReview(
	ctx context.Context,
	analyzed PullRequest,
) (*ExistingReview, error) {
	marker := fmt.Sprintf("<!-- rob-reviewer:v1 pr=%d head=%s -->", analyzed.Number, analyzed.HeadSHA)
	pending, err := publisher.client.FindPendingReview(ctx, publisher.owner, publisher.repo, analyzed.Number)
	if err != nil {
		return nil, fmt.Errorf("find pending automatic review: %w", err)
	}
	if pending != nil && !strings.Contains(pending.Body, marker) {
		if err := publisher.client.DeletePendingReview(ctx, publisher.owner, publisher.repo, analyzed.Number, pending.ID); err != nil {
			return nil, fmt.Errorf("delete stale pending review %d: %w", pending.ID, err)
		}
	}
	existing, err := publisher.client.FindReviewMarker(ctx, publisher.owner, publisher.repo, analyzed.Number, marker)
	if err != nil {
		return nil, fmt.Errorf("find automatic review marker: %w", err)
	}
	return existing, nil
}

func (publisher *Publisher) validatePublicationTarget(analyzed, current PullRequest) error {
	if current.HasAnyLabel(publisher.skipLabels) {
		return fmt.Errorf("%w: pull request %d has a configured label", ErrPublicationSuppressed, analyzed.Number)
	}
	if strings.EqualFold(current.State, "closed") && !publisher.allowClosed {
		return fmt.Errorf("%w: pull request %d is closed", ErrPublicationSuppressed, analyzed.Number)
	}
	return validatePublicationTarget(analyzed, current)
}

func publicationApprovalMarker(result Result) string {
	ids := make([]string, 0, len(result.Findings))
	for _, finding := range result.Findings {
		ids = append(ids, finding.ID)
	}
	slices.Sort(ids)
	digest := sha256.Sum256([]byte(strings.Join(ids, "\x00")))
	return fmt.Sprintf(
		"<!-- rob-reviewer-approval:v1 selection=%X -->",
		digest[:6],
	)
}

func buildReviewRequest(result Result, marker string) ReviewRequest {
	comments := make([]Comment, 0, len(result.Findings))
	for _, finding := range result.Findings {
		comments = append(comments, Comment{
			Path: finding.Path,
			Line: finding.Line,
			Side: finding.Side,
			Body: fmt.Sprintf(
				"%s\n\n%s\n\n%s\n\n**Suggested fix:** %s\n\n%s",
				experimentalReviewPrefix,
				SanitizeMarkdownTextWithCodeSpans(finding.ChangedBehavior),
				SanitizeMarkdownTextWithCodeSpans(finding.Impact),
				SanitizeMarkdownTextWithCodeSpans(finding.Recommendation),
				generatedDisclosure,
			),
		})
	}

	var summary strings.Builder
	summary.WriteString(experimentalReviewPrefix)
	summary.WriteString("\n\nHuman-approved experimental performance review.\n\n")
	fmt.Fprintf(&summary, "%s\n\n%s", generatedDisclosure, marker)

	return ReviewRequest{
		CommitID: result.PullRequest.HeadSHA,
		Event:    "",
		Body:     summary.String(),
		Comments: comments,
	}
}

func FormatStatsMarkdown(stats Stats) string {
	var output strings.Builder
	output.WriteString("<details>\n<summary>Review statistics</summary>\n\n")
	fmt.Fprintf(&output, "- Model: `%s`\n", SanitizeMarkdownText(stats.Model))
	if len(stats.ActualModels) > 0 {
		fmt.Fprintf(&output, "- Actual model calls: `%s`\n", joinSanitized(stats.ActualModels))
	}
	fmt.Fprintf(&output, "- Reasoning effort: `%s`\n", SanitizeMarkdownText(stats.ReasoningEffort))
	if len(stats.ActualReasoningEfforts) > 0 {
		fmt.Fprintf(&output, "- Actual reasoning effort: `%s`\n", joinSanitized(stats.ActualReasoningEfforts))
	}
	if len(stats.APIEndpoints) > 0 {
		fmt.Fprintf(&output, "- Model API endpoint: `%s`\n", joinSanitized(stats.APIEndpoints))
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
	output.WriteString("\n")
	output.WriteString("- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.\n")
	output.WriteString("\n</details>")
	return output.String()
}

func joinSanitized(values []string) string {
	sanitized := make([]string, 0, len(values))
	for _, value := range values {
		sanitized = append(sanitized, SanitizeMarkdownText(value))
	}
	return strings.Join(sanitized, "`, `")
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
	if current.HeadSHA != analyzed.HeadSHA {
		return fmt.Errorf("pull request head changed from %s to %s during review", analyzed.HeadSHA, current.HeadSHA)
	}
	switch {
	case strings.EqualFold(current.State, "open"):
		if current.Draft {
			return fmt.Errorf("pull request %d became a draft during review", analyzed.Number)
		}
		if current.BaseSHA != analyzed.BaseSHA {
			return fmt.Errorf("pull request base changed from %s to %s during review", analyzed.BaseSHA, current.BaseSHA)
		}
	case strings.EqualFold(current.State, "closed"):
		// The base branch can advance after a PR closes, while its reviewed head remains immutable.
	default:
		return fmt.Errorf("pull request %d has unsupported state %q", analyzed.Number, current.State)
	}
	return nil
}

func SanitizeMarkdownText(value string) string {
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
	).Replace(stripUnsafeMarkdownCharacters(value))
}

func SanitizeMarkdownTextWithCodeSpans(value string) string {
	var output strings.Builder
	plainStart := 0
	for cursor := 0; cursor < len(value); {
		if value[cursor] != '`' {
			cursor++
			continue
		}
		runEnd := backtickRunEnd(value, cursor)
		if runEnd-cursor != 1 {
			cursor = runEnd
			continue
		}
		close := nextSingleBacktick(value, runEnd)
		if close < 0 {
			cursor = runEnd
			continue
		}
		code := value[runEnd:close]
		if code == "" || strings.ContainsAny(code, "\r\n") {
			cursor = runEnd
			continue
		}
		output.WriteString(SanitizeMarkdownText(value[plainStart:cursor]))
		output.WriteByte('`')
		output.WriteString(stripUnsafeMarkdownCharacters(code))
		output.WriteByte('`')
		plainStart = close + 1
		cursor = plainStart
	}
	output.WriteString(SanitizeMarkdownText(value[plainStart:]))
	return output.String()
}

func nextSingleBacktick(value string, start int) int {
	for cursor := start; cursor < len(value); {
		if value[cursor] != '`' {
			cursor++
			continue
		}
		runEnd := backtickRunEnd(value, cursor)
		if runEnd-cursor == 1 {
			return cursor
		}
		cursor = runEnd
	}
	return -1
}

func backtickRunEnd(value string, start int) int {
	end := start
	for end < len(value) && value[end] == '`' {
		end++
	}
	return end
}

func stripUnsafeMarkdownCharacters(value string) string {
	var clean strings.Builder
	clean.Grow(len(value))
	for _, character := range value {
		if character == '\n' || character == '\t' {
			clean.WriteRune(character)
			continue
		}
		if unicode.IsControl(character) ||
			unicode.Is(unicode.Cf, character) ||
			unicode.Is(unicode.Zl, character) ||
			unicode.Is(unicode.Zp, character) {
			continue
		}
		clean.WriteRune(character)
	}
	return clean.String()
}
