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
		Focus:          "performance-review",
		Path:           "src/file.ts",
		Side:           SideRight,
		Line:           line,
		Severity:       SeverityHigh,
		Confidence:     confidence,
		Title:          title,
		Impact:         "Blocks the renderer on every scroll event.",
		Evidence:       "The changed loop executes once for each historical item.",
		Recommendation: "Batch the presentation update after reconstruction.",
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
