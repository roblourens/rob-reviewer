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
	for _, finding := range result {
		if !strings.HasPrefix(finding.ID, "PERF-") || len(finding.ID) != len("PERF-")+12 {
			t.Fatalf("invalid finding ID %q", finding.ID)
		}
		if finding.ID != FindingID(finding) {
			t.Fatalf("finding ID %q is not deterministic", finding.ID)
		}
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

func TestSelectFindingsUsesExplicitIDs(t *testing.T) {
	pull := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head"}
	first := validFinding("First issue", 0.99, 5)
	first.ID = FindingIDForPullRequest(pull, first)
	second := validFinding("Second issue", 0.98, 6)
	second.ID = FindingIDForPullRequest(pull, second)
	result := Result{PullRequest: pull, Findings: []Finding{first, second}}

	selected, err := SelectFindings(result, []string{second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Findings) != 1 || selected.Findings[0].ID != second.ID {
		t.Fatalf("selected findings = %+v", selected.Findings)
	}
}

func TestSelectFindingsRejectsUnknownID(t *testing.T) {
	pull := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head"}
	finding := validFinding("Known issue", 0.99, 5)
	finding.ID = FindingIDForPullRequest(pull, finding)
	_, err := SelectFindings(Result{PullRequest: pull, Findings: []Finding{finding}}, []string{"PERF-000000000000"})
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Fatalf("expected unknown finding ID error, got %v", err)
	}
}

func TestSelectFindingsRejectsTamperedContentAndDuplicateReportIDs(t *testing.T) {
	pull := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head"}
	finding := validFinding("Known issue", 0.99, 5)
	finding.ID = FindingIDForPullRequest(pull, finding)

	tampered := finding
	tampered.Evidence = "Changed after approval."
	if _, err := SelectFindings(Result{PullRequest: pull, Findings: []Finding{tampered}}, []string{finding.ID}); err == nil ||
		!strings.Contains(err.Error(), "does not match its content") {
		t.Fatalf("expected tampered content error, got %v", err)
	}

	nonCanonical := finding
	nonCanonical.ID = strings.ToLower(nonCanonical.ID)
	if _, err := SelectFindings(Result{PullRequest: pull, Findings: []Finding{nonCanonical}}, []string{finding.ID}); err == nil ||
		!strings.Contains(err.Error(), "not canonical uppercase") {
		t.Fatalf("expected canonical ID error, got %v", err)
	}

	if _, err := SelectFindings(Result{PullRequest: pull, Findings: []Finding{finding, finding}}, []string{finding.ID}); err == nil ||
		!strings.Contains(err.Error(), "duplicate finding ID") {
		t.Fatalf("expected duplicate report ID error, got %v", err)
	}
}

func TestTargetBoundFindingIDAuthenticatesPullRequestIdentity(t *testing.T) {
	finding := validFinding("Known issue", 0.99, 5)
	pull := PullRequest{Number: 7, BaseSHA: "base", HeadSHA: "head"}
	baseID := FindingIDForPullRequest(pull, finding)
	mutations := map[string]func(*PullRequest){
		"number":   func(changed *PullRequest) { changed.Number++ },
		"base SHA": func(changed *PullRequest) { changed.BaseSHA = "other-base" },
		"head SHA": func(changed *PullRequest) { changed.HeadSHA = "other-head" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := pull
			mutate(&changed)
			if changedID := FindingIDForPullRequest(changed, finding); changedID == baseID {
				t.Fatalf("target mutation did not change finding ID %q", baseID)
			}
		})
	}
}

func TestFindingIDAuthenticatesPublishedFindingFields(t *testing.T) {
	base := validFinding("Known issue", 0.99, 5)
	baseID := FindingID(base)
	mutations := map[string]func(*Finding){
		"focus":                func(finding *Finding) { finding.Focus = "other-focus" },
		"path":                 func(finding *Finding) { finding.Path = "src/other.ts" },
		"side":                 func(finding *Finding) { finding.Side = SideLeft },
		"line":                 func(finding *Finding) { finding.Line++ },
		"severity":             func(finding *Finding) { finding.Severity = SeverityCritical },
		"confidence":           func(finding *Finding) { finding.Confidence = 0.98 },
		"confidence rationale": func(finding *Finding) { finding.ConfidenceRationale += " More evidence." },
		"category":             func(finding *Finding) { finding.PerformanceCategory = "cpu" },
		"resource":             func(finding *Finding) { finding.PerformanceResource += " CPU" },
		"scaling":              func(finding *Finding) { finding.PerformanceScaling += " per window" },
		"outcome":              func(finding *Finding) { finding.PerformanceOutcome += " under load" },
		"causality":            func(finding *Finding) { finding.ChangeCausality = "materially-amplified" },
		"previous behavior":    func(finding *Finding) { finding.PreviousBehavior += " Previously." },
		"changed behavior":     func(finding *Finding) { finding.ChangedBehavior += " Now." },
		"causal evidence":      func(finding *Finding) { finding.CausalDiffEvidence += " Added call." },
		"title":                func(finding *Finding) { finding.Title += " now" },
		"impact":               func(finding *Finding) { finding.Impact += " under load" },
		"evidence":             func(finding *Finding) { finding.Evidence += " Direct trace." },
		"recommendation":       func(finding *Finding) { finding.Recommendation += " Coalesce calls." },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if changedID := FindingID(changed); changedID == baseID {
				t.Fatalf("field mutation did not change finding ID %q", baseID)
			}
		})
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
