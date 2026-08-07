package copilot

import (
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type allAnchors struct{}

func (allAnchors) ValidAnchor(string, review.Side, int) bool {
	return true
}

func TestCollectorRequiresAllFocuses(t *testing.T) {
	pipeline := review.NewPipeline([]string{"performance-review", "protocol-review"}, 0.85, 10, allAnchors{})
	collector := newCollector([]string{"performance-review", "protocol-review"}, pipeline)

	_, err := collector.complete([]string{"performance-review"}, "done")
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("expected focus mismatch, got %v", err)
	}
	if err := collector.completionError(); err == nil {
		t.Fatal("collector unexpectedly completed")
	}
}

func TestCollectorAcceptsValidFinding(t *testing.T) {
	pipeline := review.NewPipeline([]string{"performance-review"}, 0.85, 10, allAnchors{})
	collector := newCollector([]string{"performance-review"}, pipeline)
	finding := review.Finding{
		Focus:          "performance-review",
		Path:           "src/file.ts",
		Side:           review.SideRight,
		Line:           10,
		Severity:       review.SeverityHigh,
		Confidence:     0.95,
		Title:          "Batch repeated updates",
		Impact:         "Scrolling blocks while hidden history is rebuilt.",
		Evidence:       "The changed loop updates presentation once per child.",
		Recommendation: "Flush one presentation update after reconstruction.",
	}

	if _, err := collector.report(finding); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.complete([]string{"performance-review"}, "Reviewed performance."); err != nil {
		t.Fatal(err)
	}
	if err := collector.completionError(); err != nil {
		t.Fatal(err)
	}
	if len(collector.findingsSnapshot()) != 1 {
		t.Fatalf("findings = %v", collector.findingsSnapshot())
	}
}

func TestAgentPromptTreatsContentAsUntrusted(t *testing.T) {
	prompt := customAgentPrompt([]string{"performance-review"})
	for _, expected := range []string{"untrusted data", "distinct review pass", "complete_review exactly once"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
}
