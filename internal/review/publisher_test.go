package review

import (
	"context"
	"strings"
	"testing"
)

type fakePublisherClient struct {
	current      PullRequest
	currents     []PullRequest
	refreshCount int
	markerExists bool
	request      ReviewRequest
	published    bool
}

func (client *fakePublisherClient) GetPullRequest(context.Context, string, string, int) (PullRequest, error) {
	if len(client.currents) > 0 {
		index := min(client.refreshCount, len(client.currents)-1)
		client.refreshCount++
		return client.currents[index], nil
	}
	return client.current, nil
}

func (client *fakePublisherClient) HasReviewMarker(context.Context, string, string, int, string) (bool, error) {
	return client.markerExists, nil
}

func (client *fakePublisherClient) PublishReview(_ context.Context, _, _ string, _ int, request ReviewRequest) error {
	client.request = request
	client.published = true
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
	if client.request.Event != "COMMENT" || client.request.CommitID != "head" {
		t.Fatalf("request = %+v", client.request)
	}
	if len(client.request.Comments) != 1 || !strings.Contains(client.request.Body, "rob-reviewer:v1") {
		t.Fatalf("request = %+v", client.request)
	}
	if !strings.Contains(client.request.Comments[0].Body, generatedDisclosure) {
		t.Fatal("inline comment is missing generated-content disclosure")
	}
	for _, expected := range []string{
		"Review statistics",
		"`gpt-5.6-sol`",
		"`high`",
		"12.345s",
		"10000 input, 2000 output, 12000 total",
		"123.456 nano-AI units",
		"model billing multiplier: 15.000",
		"USD cost: unavailable",
	} {
		if !strings.Contains(client.request.Body, expected) {
			t.Fatalf("review body missing %q: %s", expected, client.request.Body)
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

func TestSanitizeMarkdownTextNeutralizesActiveContent(t *testing.T) {
	result := SanitizeMarkdownText("@team ![image](https://example.test/x) <details>\x00")
	for _, forbidden := range []string{"@team", "![", "<details>", "\x00"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("sanitized text %q contains %q", result, forbidden)
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
