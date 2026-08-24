package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/review"
)

type fakeSavedReportClient struct {
	pull      review.PullRequest
	published review.ReviewRequest
	submitted int64
}

func (client *fakeSavedReportClient) GetPullRequest(context.Context, string, string, int) (review.PullRequest, error) {
	return client.pull, nil
}

func (client *fakeSavedReportClient) FindReviewMarker(
	context.Context,
	string,
	string,
	int,
	string,
) (*review.ExistingReview, error) {
	return nil, nil
}

func (client *fakeSavedReportClient) CreatePendingReview(
	_ context.Context,
	_, _ string,
	_ int,
	request review.ReviewRequest,
) (int64, error) {
	client.published = request
	return 42, nil
}

func (client *fakeSavedReportClient) SubmitPendingReview(
	_ context.Context,
	_, _ string,
	_ int,
	reviewID int64,
) error {
	client.submitted = reviewID
	return nil
}

func TestWriteAndLoadResultFiles(t *testing.T) {
	result := savedResult()
	paths, err := WriteResultFiles(t.TempDir(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing result file %q: %v", path, err)
		}
	}
	loaded, err := LoadResult(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Findings[0].ID != result.Findings[0].ID {
		t.Fatalf("loaded result = %+v", loaded)
	}
	markdown, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(markdown) == "" {
		t.Fatal("empty Markdown report")
	}
}

func TestSavedReportPublisherPublishesOnlyApprovedFinding(t *testing.T) {
	result := savedResult()
	second := result.Findings[0]
	second.Title = "Second issue"
	second.ID = review.FindingIDForPullRequest(result.PullRequest, second)
	result.Findings = append(result.Findings, second)
	directory := t.TempDir()
	paths, err := WriteResultFiles(directory, result)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeSavedReportClient{pull: result.PullRequest}
	publisher := &SavedReportPublisher{
		config: configForTest(),
		client: client,
	}

	selected, published, err := publisher.Publish(context.Background(), paths[0], []string{second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !published || len(selected.Findings) != 1 || selected.Findings[0].ID != second.ID {
		t.Fatalf("selected = %+v, published = %v", selected, published)
	}
	if len(client.published.Comments) != 1 || client.published.Comments[0].Path != second.Path {
		t.Fatalf("published request = %+v", client.published)
	}
	if client.submitted != 42 {
		t.Fatalf("submitted review ID = %d", client.submitted)
	}
}

func TestLoadResultRejectsInvalidReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"Findings":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResult(path); err == nil {
		t.Fatal("expected missing PR identity error")
	}
}

func TestLoadResultRejectsTamperedFinding(t *testing.T) {
	result := savedResult()
	result.Findings[0].Recommendation = "Publish altered advice."
	path := writeResultJSON(t, result)

	if _, err := LoadResult(path); err == nil || !strings.Contains(err.Error(), "does not match its content") {
		t.Fatalf("expected tampered finding error, got %v", err)
	}
}

func TestLoadResultRejectsTamperedPullRequestIdentity(t *testing.T) {
	result := savedResult()
	result.PullRequest.Number++
	path := writeResultJSON(t, result)

	if _, err := LoadResult(path); err == nil || !strings.Contains(err.Error(), "does not match its content") {
		t.Fatalf("expected target identity error, got %v", err)
	}
}

func TestLoadResultRejectsDuplicateFindingIDs(t *testing.T) {
	result := savedResult()
	result.Findings = append(result.Findings, result.Findings[0])
	path := writeResultJSON(t, result)

	if _, err := LoadResult(path); err == nil || !strings.Contains(err.Error(), "duplicate finding ID") {
		t.Fatalf("expected duplicate ID error, got %v", err)
	}
}

func TestLoadResultRejectsUnknownAndTrailingContent(t *testing.T) {
	result := savedResult()
	content, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string][]byte{
		"unknown":  append(content[:len(content)-1], []byte(`,"unexpected":true}`)...),
		"trailing": append(append(content, '\n'), []byte(`{"second":true}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.json")
			if err := os.WriteFile(path, invalid, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadResult(path); err == nil {
				t.Fatal("expected invalid saved report error")
			}
		})
	}
}

func TestLoadResultRejectsOversizedReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, make([]byte, maxSavedReportBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResult(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized report error, got %v", err)
	}
}

func savedResult() review.Result {
	pull := review.PullRequest{
		Number:            7,
		URL:               "https://github.com/microsoft/vscode/pull/7",
		State:             "open",
		AuthorAssociation: "MEMBER",
		BaseSHA:           "base",
		HeadSHA:           "head1234567890",
	}
	result := review.Result{
		PullRequest: pull,
		Findings: []review.Finding{{
			Focus:               "performance-review",
			Path:                "src/file.ts",
			Side:                review.SideRight,
			Line:                12,
			Severity:            review.SeverityHigh,
			Confidence:          0.95,
			ConfidenceRationale: "Directly traced.",
			PerformanceCategory: "latency",
			PerformanceResource: "CPU",
			PerformanceScaling:  "Per item",
			PerformanceOutcome:  "Delay",
			ChangeCausality:     "introduced",
			PreviousBehavior:    "No work",
			ChangedBehavior:     "Repeated work",
			CausalDiffEvidence:  "Added loop",
			Title:               "Avoid repeated work",
			Impact:              "Blocks interaction.",
			Evidence:            "The loop is on the path.",
			Recommendation:      "Batch it.",
		}},
	}
	result.Findings[0].ID = review.FindingIDForPullRequest(result.PullRequest, result.Findings[0])
	return result
}

func writeResultJSON(t *testing.T, result review.Result) string {
	t.Helper()
	content, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func configForTest() config.Config {
	return config.Config{
		Target: config.TargetConfig{
			Owner: "microsoft",
			Repo:  "vscode",
		},
	}
}
