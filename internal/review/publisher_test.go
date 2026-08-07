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
	pull := PullRequest{Number: 7, HeadSHA: "head", State: "open"}
	client := &fakePublisherClient{current: pull}
	publisher := NewPublisher(client, "microsoft", "vscode")
	result := Result{
		PullRequest: pull,
		Findings: []Finding{{
			Focus:          "performance-review",
			Path:           "src/file.ts",
			Side:           SideRight,
			Line:           12,
			Severity:       SeverityHigh,
			Confidence:     0.98,
			Title:          "Batch repeated work",
			Impact:         "The loop blocks scrolling.",
			Evidence:       "It runs for every historical item.",
			Recommendation: "Flush one update after reconstruction.",
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
}

func TestPublisherRejectsStaleHead(t *testing.T) {
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "new", State: "open"}}
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
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "head", State: "open"}, markerExists: true}
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
	client := &fakePublisherClient{current: PullRequest{Number: 7, HeadSHA: "new", State: "open"}}
	publisher := NewPublisher(client, "microsoft", "vscode")

	_, err := publisher.Publish(context.Background(), Result{
		PullRequest: PullRequest{Number: 7, HeadSHA: "old"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "head changed") {
		t.Fatalf("expected stale clean-review error, got %v", err)
	}
}

func TestSanitizeMarkdownTextNeutralizesActiveContent(t *testing.T) {
	result := sanitizeMarkdownText("@team ![image](https://example.test/x) <details>\x00")
	for _, forbidden := range []string{"@team", "![", "<details>", "\x00"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("sanitized text %q contains %q", result, forbidden)
		}
	}
}

func TestPublisherRevalidatesImmediatelyBeforePost(t *testing.T) {
	analyzed := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head", State: "open"}
	client := &fakePublisherClient{
		currents: []PullRequest{
			analyzed,
			{Number: 7, BaseSHA: "base", HeadSHA: "new-head", State: "open"},
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
