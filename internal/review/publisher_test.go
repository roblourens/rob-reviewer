package review

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakePublisherClient struct {
	current           PullRequest
	currents          []PullRequest
	refreshCount      int
	markerExists      bool
	markerReview      *ExistingReview
	markerAfterSubmit *ExistingReview
	pendingReview     *ExistingReview
	reviewComments    []Comment
	request           ReviewRequest
	pendingID         int64
	submittedID       int64
	submitAttempted   bool
	published         bool
	createErr         error
	submitErr         error
	deletedID         int64
}

func (client *fakePublisherClient) GetPullRequest(context.Context, string, string, int) (PullRequest, error) {
	if len(client.currents) > 0 {
		index := min(client.refreshCount, len(client.currents)-1)
		client.refreshCount++
		return client.currents[index], nil
	}
	return client.current, nil
}

func (client *fakePublisherClient) FindReviewMarker(context.Context, string, string, int, string) (*ExistingReview, error) {
	if client.markerReview != nil {
		return client.markerReview, nil
	}
	if client.submitAttempted && client.markerAfterSubmit != nil {
		return client.markerAfterSubmit, nil
	}

	if !client.markerExists {
		return nil, nil
	}
	return &ExistingReview{ID: 7, State: "COMMENTED"}, nil
}

func (client *fakePublisherClient) FindPendingReview(context.Context, string, string, int) (*ExistingReview, error) {
	return client.pendingReview, nil
}

func (client *fakePublisherClient) GetReviewComments(context.Context, string, string, int, int64) ([]Comment, error) {
	return client.reviewComments, nil
}

func (client *fakePublisherClient) CreatePendingReview(
	_ context.Context,
	_, _ string,
	_ int,
	request ReviewRequest,
) (int64, error) {
	if client.createErr != nil {
		return 0, client.createErr
	}
	client.request = request
	client.pendingID = 42
	return client.pendingID, nil
}

func (client *fakePublisherClient) SubmitPendingReview(
	_ context.Context,
	_, _ string,
	_ int,
	reviewID int64,
) error {
	client.submitAttempted = true
	if client.submitErr != nil {
		return client.submitErr
	}

	client.submittedID = reviewID
	client.published = true
	return nil
}

func (client *fakePublisherClient) DeletePendingReview(
	_ context.Context,
	_, _ string,
	_ int,
	reviewID int64,
) error {
	client.deletedID = reviewID
	return nil
}

func TestPublisherCreatesCommentReview(t *testing.T) {
	pull := PullRequest{Number: 7, HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}
	client := &fakePublisherClient{current: pull}
	publisher := NewPublisher(client, "microsoft", "vscode")
	result := Result{
		PullRequest: pull,
		Stats: Stats{
			Model:                   "gpt-5.6-sol",
			ActualModels:            []string{"gpt-5.6-sol"},
			ReasoningEffort:         "high",
			ActualReasoningEfforts:  []string{"high"},
			APIEndpoints:            []string{"/v1/messages"},
			WallClockMilliseconds:   12_345,
			ModelCalls:              3,
			InputTokens:             10_000,
			OutputTokens:            2_000,
			TotalTokens:             12_000,
			ReasoningTokens:         500,
			CacheReadTokens:         4_000,
			CacheWriteTokens:        1_000,
			APIDurationMilliseconds: 10_500,
			ToolCalls:               7,
			NanoAIUnits:             123.456,
			ModelBillingMultipliers: []float64{15},
		},
		Findings: []Finding{{
			ID:                  "PERF-1234567890AB",
			Focus:               "performance-review",
			Path:                "src/file.ts",
			Side:                SideRight,
			Line:                12,
			Severity:            SeverityHigh,
			Confidence:          0.98,
			ConfidenceRationale: "The changed call is directly inside the repeated reconstruction loop.",
			PerformanceCategory: "latency",
			PerformanceResource: "Synchronous renderer CPU work.",
			PerformanceScaling:  "One expensive update per historical item.",
			PerformanceOutcome:  "Scroll latency and dropped frames.",
			ChangeCausality:     "introduced",
			PreviousBehavior:    "The prior renderer did not repeat this work for each child.",
			ChangedBehavior:     "The diff repeats the work for every historical child.",
			CausalDiffEvidence:  "The added changed line is called inside the child loop.",
			Title:               "Batch repeated work",
			Impact:              "The loop blocks scrolling.",
			Evidence:            "It runs for every historical item.",
			Recommendation:      "Flush one update after reconstruction.",
		}},
	}

	published, err := publisher.Publish(context.Background(), result, false)
	if err != nil {
		t.Fatal(err)
	}
	if !published || !client.published {
		t.Fatal("expected publication")
	}
	if client.request.Event != "" || client.request.CommitID != "head" {
		t.Fatalf("request = %+v", client.request)
	}
	if client.pendingID != 42 || client.submittedID != 42 {
		t.Fatalf("pending=%d submitted=%d", client.pendingID, client.submittedID)
	}
	if len(client.request.Comments) != 1 || !strings.Contains(client.request.Body, "rob-reviewer:v1") {
		t.Fatalf("request = %+v", client.request)
	}
	if !strings.Contains(client.request.Comments[0].Body, generatedDisclosure) {
		t.Fatal("inline comment is missing generated-content disclosure")
	}
	if !strings.HasPrefix(client.request.Comments[0].Body, experimentalReviewPrefix) {
		t.Fatal("inline comment is missing experimental bot prefix")
	}
	if !strings.HasPrefix(client.request.Body, experimentalReviewPrefix) {
		t.Fatal("review summary is missing experimental bot prefix")
	}
	for _, expected := range []string{
		"**Severity: high**",
		"The diff repeats the work for every historical child.",
		"The loop blocks scrolling.",
		"**Suggested fix:** Flush one update after reconstruction.",
	} {
		if !strings.Contains(client.request.Comments[0].Body, expected) {
			t.Fatalf("inline comment missing %q: %s", expected, client.request.Comments[0].Body)
		}
	}
	combined := client.request.Body + client.request.Comments[0].Body
	for _, forbidden := range []string{
		"PERF-1234567890AB",
		"Confidence",
		"Review statistics",
		"gpt-5.6-sol",
		"high-confidence",
		"Causal diff evidence",
		"Performance mechanism",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("public review contains %q: %s", forbidden, combined)
		}
	}
}

func TestPublisherRejectsStaleHead(t *testing.T) {
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "new", State: "open", AuthorAssociation: "MEMBER"}}
	publisher := NewPublisher(client, "microsoft", "vscode")

	_, err := publisher.Publish(context.Background(), Result{
		PullRequest: PullRequest{Number: 7, HeadSHA: "old"},
		Findings:    []Finding{{Title: "Finding"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "head changed") {
		t.Fatalf("expected stale head error, got %v", err)
	}
}

func TestPublisherResumesOnlyMatchingPendingReview(t *testing.T) {
	pull := PullRequest{Number: 7, HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}
	result := Result{
		PullRequest: pull,
		Findings:    []Finding{{ID: "PERF-1234567890AB"}},
	}
	baseMarker := "<!-- rob-reviewer:v1 pr=7 head=head -->"
	client := &fakePublisherClient{
		current: pull,
		markerReview: &ExistingReview{
			ID:    77,
			State: "PENDING",
			Body:  baseMarker + "\n" + publicationApprovalMarker(result),
		},
	}
	publisher := NewPublisher(client, "microsoft", "vscode")

	published, err := publisher.Publish(context.Background(), result, false)
	if err != nil || !published || client.submittedID != 77 || client.pendingID != 0 {
		t.Fatalf("published=%v submitted=%d pending=%d err=%v", published, client.submittedID, client.pendingID, err)
	}

	client.submittedID = 0
	client.markerReview.Body = baseMarker + "\n<!-- rob-reviewer-approval:v1 selection=000000000000 -->"
	if _, err := publisher.Publish(context.Background(), result, false); err == nil ||
		!strings.Contains(err.Error(), "different approved finding set") {
		t.Fatalf("expected mismatched pending review error, got %v", err)
	}
}

func TestPublisherSurfacesPendingReviewFailures(t *testing.T) {
	pull := PullRequest{Number: 7, HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}
	result := Result{PullRequest: pull, Findings: []Finding{{ID: "PERF-1234567890AB"}}}

	client := &fakePublisherClient{current: pull, createErr: errors.New("pending conflict")}
	if _, err := NewPublisher(client, "microsoft", "vscode").Publish(context.Background(), result, false); err == nil ||
		!strings.Contains(err.Error(), "create pending performance review claim") {
		t.Fatalf("expected pending creation error, got %v", err)
	}

	client = &fakePublisherClient{current: pull, submitErr: errors.New("submit failed")}
	if _, err := NewPublisher(client, "microsoft", "vscode").Publish(context.Background(), result, false); err == nil ||
		!strings.Contains(err.Error(), "submit pending performance review") {
		t.Fatalf("expected pending submission error, got %v", err)
	}
}

func TestPublisherConfirmsAmbiguousPendingReviewSubmission(t *testing.T) {
	pull := PullRequest{Number: 7, HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}
	result := Result{PullRequest: pull, Findings: []Finding{{ID: "PERF-1234567890AB"}}}
	client := &fakePublisherClient{
		current:           pull,
		submitErr:         errors.New("response read failed"),
		markerAfterSubmit: &ExistingReview{ID: 42, State: "COMMENTED"},
	}
	published, err := NewPublisher(client, "microsoft", "vscode").Publish(context.Background(), result, false)
	if err != nil || !published {
		t.Fatalf("published=%v err=%v", published, err)
	}
}

func TestPublisherSkipsCleanAndDuplicateReviews(t *testing.T) {
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}, markerExists: true}
	publisher := NewPublisher(client, "microsoft", "vscode")

	published, err := publisher.Publish(context.Background(), Result{PullRequest: client.current}, false)
	if err != nil || published {
		t.Fatalf("clean result: published = %v, err = %v", published, err)
	}

	published, err = publisher.Publish(context.Background(), Result{
		PullRequest: client.current,
		Findings:    []Finding{{Title: "Duplicate"}},
	}, false)
	if err != nil || published || client.published {
		t.Fatalf("duplicate result: published = %v, err = %v", published, err)
	}
}

func TestPublisherRejectsStaleHeadForCleanResult(t *testing.T) {
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "new", State: "open", AuthorAssociation: "MEMBER"}}
	publisher := NewPublisher(client, "microsoft", "vscode")

	_, err := publisher.Publish(context.Background(), Result{
		PullRequest: PullRequest{Number: 7, HeadSHA: "old"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "head changed") {
		t.Fatalf("expected stale clean-review error, got %v", err)
	}
}

func TestPublisherAllowsClosedPullRequestWithAdvancedBase(t *testing.T) {
	analyzed := PullRequest{
		Number:            7,
		BaseSHA:           "original-base",
		HeadSHA:           "head",
		State:             "open",
		AuthorAssociation: "MEMBER",
	}
	client := &fakePublisherClient{
		current: PullRequest{
			Number:            7,
			BaseSHA:           "advanced-base",
			HeadSHA:           "head",
			State:             "closed",
			AuthorAssociation: "MEMBER",
		},
	}
	published, err := NewPublisher(client, "microsoft", "vscode").Publish(context.Background(), Result{
		PullRequest: analyzed,
		Findings:    []Finding{{ID: "PERF-1234567890AB"}},
	}, false)
	if err != nil || !published || !client.published {
		t.Fatalf("published=%v client.published=%v err=%v", published, client.published, err)
	}
}

func TestPublisherRejectsSuppressionLabelAndResumesPendingReview(t *testing.T) {
	pull := PullRequest{
		Number:            7,
		BaseSHA:           "base",
		HeadSHA:           "head",
		State:             "open",
		AuthorAssociation: "MEMBER",
		Labels:            []string{"performance-reviewer:skip"},
	}
	client := &fakePublisherClient{current: pull}
	publisher := NewPublisher(client, "microsoft", "vscode", "performance-reviewer:skip")
	if _, err := publisher.Publish(context.Background(), Result{
		PullRequest: pull,
		Findings:    []Finding{{ID: "PERF-1234567890AB"}},
	}, false); err == nil || !errors.Is(err, ErrPublicationSuppressed) {
		t.Fatalf("expected suppression error, got %v", err)
	}

	pull.Labels = nil
	client.current = pull
	client.reviewComments = []Comment{{Path: "src/file.ts", Line: 3, Side: SideRight, Body: "comment"}}
	comments, err := publisher.ResumePending(context.Background(), pull, 77)
	if err != nil {
		t.Fatal(err)
	}
	if client.submittedID != 77 || len(comments) != 1 || comments[0].Body != "comment" {
		t.Fatalf("submitted review = %d, comments = %+v", client.submittedID, comments)
	}

	client.submitErr = errors.New("response read failed")
	client.markerAfterSubmit = &ExistingReview{ID: 77, State: "COMMENTED"}
	comments, err = publisher.ResumePending(context.Background(), pull, 77)
	if err != nil || len(comments) != 1 {
		t.Fatalf("confirmed comments = %+v, err = %v", comments, err)
	}

	pull.Labels = []string{"performance-reviewer:skip"}
	client.current = pull
	if _, err := publisher.ResumePending(context.Background(), pull, 88); !errors.Is(err, ErrPublicationSuppressed) {
		t.Fatalf("expected suppressed pending review, got %v", err)
	}
	if client.deletedID != 88 {
		t.Fatalf("deleted review = %d", client.deletedID)
	}
}

func TestAutomaticPublisherRejectsClosedPRAndDeletesStalePendingReview(t *testing.T) {
	closed := PullRequest{
		Number:            7,
		BaseSHA:           "base",
		HeadSHA:           "new-head",
		State:             "closed",
		AuthorAssociation: "MEMBER",
	}
	client := &fakePublisherClient{
		current: closed,
		pendingReview: &ExistingReview{
			ID:    91,
			State: "PENDING",
			Body:  "<!-- rob-reviewer:v1 pr=7 head=old-head -->",
		},
	}
	publisher := NewAutomaticPublisher(client, "microsoft", "vscode")
	existing, err := publisher.PrepareAutomaticReview(context.Background(), closed)
	if err != nil {
		t.Fatal(err)
	}
	if existing != nil || client.deletedID != 91 {
		t.Fatalf("existing=%+v deleted=%d", existing, client.deletedID)
	}
	if _, err := publisher.Publish(context.Background(), Result{
		PullRequest: closed,
		Findings:    []Finding{{ID: "PERF-1234567890AB"}},
	}, false); !errors.Is(err, ErrPublicationSuppressed) {
		t.Fatalf("expected closed automatic suppression, got %v", err)
	}

	open := closed
	open.State = "open"
	client.current = open
	client.deletedID = 0
	client.pendingReview = &ExistingReview{
		ID:    92,
		State: "PENDING",
		Body:  "<!-- rob-reviewer:v1 pr=7 head=new-head -->\n" + humanApprovedSummary,
	}
	existing, err = publisher.PrepareAutomaticReview(context.Background(), open)
	if err != nil {
		t.Fatal(err)
	}
	if existing != nil || client.deletedID != 92 {
		t.Fatalf("legacy pending review was not replaced: existing=%+v deleted=%d", existing, client.deletedID)
	}
}

func TestSanitizeMarkdownTextNeutralizesActiveContent(t *testing.T) {
	result := SanitizeMarkdownText("@team ![image](https://example.test/x) <details>\x00\u202e\u2066\u200b\u0085\u2028")
	for _, forbidden := range []string{"@team", "![", "<details>", "\x00", "\u202e", "\u2066", "\u200b", "\u0085", "\u2028"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("sanitized text %q contains %q", result, forbidden)
		}
	}
}

func TestSanitizeMarkdownTextWithCodeSpansPreservesOnlyInlineCode(t *testing.T) {
	result := SanitizeMarkdownTextWithCodeSpans(
		"Call `publicLog2` before @team <script> ![image](url); reject ``fences`` and `multiline\ncode`.",
	)
	if !strings.Contains(result, "Call `publicLog2`") {
		t.Fatalf("sanitized text did not preserve code span: %q", result)
	}
	for _, forbidden := range []string{"@team", "<script>", "![image]", "``fences``", "`multiline\ncode`"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("sanitized text %q contains %q", result, forbidden)
		}
	}
}

func TestReviewRequestSanitizesAllDynamicMetadata(t *testing.T) {
	result := Result{
		PullRequest: PullRequest{Number: 7, HeadSHA: "head"},
		Findings: []Finding{{
			ID:                  "PERF-1234567890AB",
			Focus:               "@focus <details>",
			Path:                "src/file.ts",
			Side:                SideRight,
			Line:                1,
			Severity:            Severity("@severity"),
			ConfidenceRationale: "rationale",
			PerformanceCategory: "latency",
			PerformanceResource: "CPU",
			PerformanceScaling:  "per item",
			PerformanceOutcome:  "delay",
			ChangeCausality:     "introduced",
			PreviousBehavior:    "before",
			ChangedBehavior:     "after `publicLog2`",
			CausalDiffEvidence:  "changed line",
			Title:               "title",
			Impact:              "@impact <script>",
			Evidence:            "evidence",
			Recommendation:      "![fix](url)",
		}},
		Stats: Stats{
			Model:                  "@model <script>",
			ActualModels:           []string{"![model](url)"},
			ReasoningEffort:        "@high",
			ActualReasoningEfforts: []string{"<details>"},
			APIEndpoints:           []string{"@endpoint"},
		},
	}
	request := buildReviewRequest(result, "<!-- marker -->", "Test review.")
	combined := request.Body + request.Comments[0].Body
	if !strings.Contains(combined, "`publicLog2`") {
		t.Fatalf("review content did not preserve code span: %s", combined)
	}
	for _, forbidden := range []string{
		"@focus",
		"@severity",
		"@model",
		"<script>",
		"![model]",
		"@high",
		"@endpoint",
		"@impact",
		"<script>",
		"![fix]",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("review content contains unsanitized %q: %s", forbidden, combined)
		}
	}
}

func TestPublisherRevalidatesImmediatelyBeforePost(t *testing.T) {
	analyzed := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head", State: "open", AuthorAssociation: "MEMBER"}
	client := &fakePublisherClient{
		currents: []PullRequest{
			analyzed,
			{Number: 7, BaseSHA: "base", HeadSHA: "new-head", State: "open", AuthorAssociation: "MEMBER"},
		},
	}

	publisher := NewPublisher(client, "microsoft", "vscode")
	_, err := publisher.Publish(context.Background(), Result{
		PullRequest: analyzed,
		Findings: []Finding{{
			Focus:          "performance-review",
			Path:           "src/file.ts",
			Side:           SideRight,
			Line:           1,
			Severity:       SeverityHigh,
			Title:          "Finding",
			Impact:         "Impact",
			Evidence:       "Evidence",
			Recommendation: "Recommendation",
		}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "head changed") || client.published {
		t.Fatalf("error = %v, published = %v", err, client.published)
	}
}

func TestPublisherRejectsNonTeamAuthorBeforePost(t *testing.T) {
	analyzed := PullRequest{
		Number:            7,
		BaseSHA:           "base",
		HeadSHA:           "head",
		State:             "open",
		AuthorLogin:       "teammate",
		AuthorAssociation: "MEMBER",
	}
	client := &fakePublisherClient{
		currents: []PullRequest{
			analyzed,
			{
				Number:            7,
				BaseSHA:           "base",
				HeadSHA:           "head",
				State:             "open",
				AuthorLogin:       "former-teammate",
				AuthorAssociation: "CONTRIBUTOR",
			},
		},
	}
	publisher := NewPublisher(client, "microsoft", "vscode")
	_, err := publisher.Publish(context.Background(), Result{
		PullRequest: analyzed,
		Findings: []Finding{{
			Focus:          "performance-review",
			Path:           "src/file.ts",
			Side:           SideRight,
			Line:           1,
			Severity:       SeverityHigh,
			Title:          "Finding",
			Impact:         "Impact",
			Evidence:       "Evidence",
			Recommendation: "Recommendation",
		}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "no longer team-authored") || client.published {
		t.Fatalf("error = %v, published = %v", err, client.published)
	}
}
