package review

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

type anchorSet map[string]struct{}

func (anchors anchorSet) ValidAnchor(path string, side Side, line int) bool {
	_, exists := anchors[anchor(path, side, line)]
	return exists
}

func anchor(path string, side Side, line int) string {
	return fmt.Sprintf("%s|%s|%d", path, side, line)
}

func validFinding(title string, confidence float64, line int) Finding {
	return Finding{
		Focus:               "performance-review",
		Path:                "src/file.ts",
		Side:                SideRight,
		Line:                line,
		Severity:            SeverityHigh,
		Confidence:          confidence,
		ConfidenceRationale: "The changed loop is directly reachable from the scroll callback and scales with history.",
		PerformanceCategory: "latency",
		PerformanceResource: "Synchronous renderer CPU work.",
		PerformanceScaling:  "One expensive update per historical item per row reconstruction.",
		PerformanceOutcome:  "Scroll latency and dropped frames.",
		ChangeCausality:     "introduced",
		PreviousBehavior:    "The row did not render a title for each historical child.",
		ChangedBehavior:     "The diff renders an expensive title for every historical child.",
		CausalDiffEvidence:  "The added loop calls the title renderer once per historical child.",
		Title:               title,
		Impact:              "Blocks the renderer on every scroll event.",
		Evidence:            "The changed loop executes once for each historical item.",
		Recommendation:      "Batch the presentation update after reconstruction.",
	}
}

func TestPipelineFiltersRanksDeduplicatesAndCaps(t *testing.T) {
	anchors := anchorSet{}
	var candidates []Finding
	for index := 1; index <= 12; index++ {
		anchors[anchor("src/file.ts", SideRight, index)] = struct{}{}
		candidates = append(candidates, validFinding("Finding "+string(rune('A'+index)), 0.80+float64(index)/100, index))
	}
	candidates = append(candidates, candidates[len(candidates)-1])
	pipeline := NewPipeline([]string{"performance-review"}, 0.85, 10, anchors)

	result, err := pipeline.Process(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 8 {
		t.Fatalf("result count = %d, want 8 after threshold and deduplication", len(result))
	}
	confidences := make([]float64, 0, len(result))
	for _, finding := range result {
		confidences = append(confidences, finding.Confidence)
	}
	if !slices.IsSortedFunc(confidences, func(left, right float64) int {
		if left > right {
			return -1
		}
		if left < right {
			return 1
		}
		return 0
	}) {
		t.Fatalf("confidences are not descending: %v", confidences)
	}
}

func TestPipelineRejectsInvalidAnchor(t *testing.T) {
	pipeline := NewPipeline([]string{"performance-review"}, 0.85, 10, anchorSet{})
	_, err := pipeline.Process([]Finding{validFinding("Invalid anchor", 0.99, 5)})
	if err == nil || !strings.Contains(err.Error(), "not a changed diff line") {
		t.Fatalf("expected changed line error, got %v", err)
	}
}

func TestPipelineRejectsCorrectnessOnlyFindingWithoutPerformanceMechanism(t *testing.T) {
	finding := validFinding("Stale button state", 0.99, 5)
	finding.PerformanceCategory = ""
	finding.PerformanceResource = ""
	finding.PerformanceScaling = ""
	finding.PerformanceOutcome = ""
	finding.Impact = "The button does not update after a visibility event."
	pipeline := NewPipeline(
		[]string{"performance-review"},
		0.85,
		10,
		anchorSet{anchor("src/file.ts", SideRight, 5): {}},
	)

	_, err := pipeline.Process([]Finding{finding})
	if err == nil || !strings.Contains(err.Error(), "performance category") {
		t.Fatalf("expected performance-only validation error, got %v", err)
	}
}

func TestPipelineRejectsPreExistingNonCriticalIssue(t *testing.T) {
	finding := validFinding("Existing file stats", 0.99, 5)
	finding.ChangeCausality = "pre-existing-critical"
	finding.Severity = SeverityLow
	pipeline := NewPipeline(
		[]string{"performance-review"},
		0.85,
		10,
		anchorSet{anchor("src/file.ts", SideRight, 5): {}},
	)

	_, err := pipeline.Process([]Finding{finding})
	if err == nil || !strings.Contains(err.Error(), "only at critical severity") {
		t.Fatalf("expected PR-scope validation error, got %v", err)
	}
}
