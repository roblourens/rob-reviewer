package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	maxTitleLength          = 120
	maxFindingSectionLength = 2_000
)

type AnchorValidator interface {
	ValidAnchor(path string, side Side, line int) bool
}

type Pipeline struct {
	focuses       map[string]struct{}
	minConfidence float64
	maxFindings   int
	anchors       AnchorValidator
}

func NewPipeline(focuses []string, minConfidence float64, maxFindings int, anchors AnchorValidator) *Pipeline {
	focusSet := make(map[string]struct{}, len(focuses))
	for _, focus := range focuses {
		focusSet[focus] = struct{}{}
	}
	return &Pipeline{
		focuses:       focusSet,
		minConfidence: minConfidence,
		maxFindings:   maxFindings,
		anchors:       anchors,
	}
}

func (pipeline *Pipeline) Process(candidates []Finding) ([]Finding, error) {
	seen := make(map[string]struct{}, len(candidates))
	result := make([]Finding, 0, len(candidates))
	var validationErrors []error
	for index, candidate := range candidates {
		candidate = normalizeFinding(candidate)
		if err := pipeline.validate(candidate); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("finding %d: %w", index+1, err))
			continue
		}
		if candidate.Confidence < pipeline.minConfidence {
			continue
		}
		candidate.ID = FindingID(candidate)
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s", candidate.Path, candidate.Side, candidate.Line, strings.ToLower(candidate.Title))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	if len(validationErrors) > 0 {
		return nil, errors.Join(validationErrors...)
	}

	slices.SortFunc(result, compareFindings)
	if len(result) > pipeline.maxFindings {
		result = result[:pipeline.maxFindings]
	}
	return result, nil
}

func FindingID(finding Finding) string {
	finding.ID = ""
	content, err := json.Marshal(finding)
	if err != nil {
		panic(fmt.Sprintf("marshal finding ID content: %v", err))
	}
	digest := sha256.Sum256(content)
	return "PERF-" + strings.ToUpper(hex.EncodeToString(digest[:6]))
}

func FindingIDForPullRequest(pull PullRequest, finding Finding) string {
	finding.ID = ""
	content, err := json.Marshal(struct {
		Number  int
		BaseSHA string
		HeadSHA string
		Finding Finding
	}{
		Number:  pull.Number,
		BaseSHA: pull.BaseSHA,
		HeadSHA: pull.HeadSHA,
		Finding: finding,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal target-bound finding ID content: %v", err))
	}
	digest := sha256.Sum256(content)
	return "PERF-" + strings.ToUpper(hex.EncodeToString(digest[:6]))
}

func BindFindingIDs(result *Result) {
	for index := range result.Findings {
		result.Findings[index].ID = FindingIDForPullRequest(result.PullRequest, result.Findings[index])
	}
}

func SelectFindings(result Result, approvedIDs []string) (Result, error) {
	if len(approvedIDs) == 0 {
		return Result{}, errors.New("at least one finding ID must be approved")
	}
	approved := make(map[string]struct{}, len(approvedIDs))
	for _, id := range approvedIDs {
		id = strings.ToUpper(strings.TrimSpace(id))
		if id == "" {
			return Result{}, errors.New("finding IDs cannot be empty")
		}
		if _, exists := approved[id]; exists {
			return Result{}, fmt.Errorf("duplicate approved finding ID %q", id)
		}
		approved[id] = struct{}{}
	}

	if err := ValidateFindingIDs(result); err != nil {
		return Result{}, err
	}
	selected := make([]Finding, 0, len(approved))
	for _, finding := range result.Findings {
		normalizedID := strings.ToUpper(finding.ID)
		if _, exists := approved[normalizedID]; exists {
			selected = append(selected, finding)
			delete(approved, normalizedID)
		}
	}
	if len(approved) > 0 {
		missing := slices.Sorted(maps.Keys(approved))
		return Result{}, fmt.Errorf("approved finding IDs not present in report: %s", strings.Join(missing, ", "))
	}
	result.Findings = selected
	return result, nil
}

func ValidateFindingIDs(result Result) error {
	seen := make(map[string]struct{}, len(result.Findings))
	for _, finding := range result.Findings {
		if finding.ID == "" {
			return errors.New("saved report contains a finding without an ID")
		}
		normalizedID := strings.ToUpper(finding.ID)
		if finding.ID != normalizedID {
			return fmt.Errorf("saved finding ID %q is not canonical uppercase", finding.ID)
		}
		if normalizedID != FindingIDForPullRequest(result.PullRequest, finding) {
			return fmt.Errorf("saved finding %q does not match its content", finding.ID)
		}
		if _, exists := seen[normalizedID]; exists {
			return fmt.Errorf("saved report contains duplicate finding ID %q", finding.ID)
		}
		seen[normalizedID] = struct{}{}
	}
	return nil
}

func (pipeline *Pipeline) validate(finding Finding) error {
	if _, exists := pipeline.focuses[finding.Focus]; !exists {
		return fmt.Errorf("unknown focus %q; enabled focuses are %v", finding.Focus, slices.Sorted(maps.Keys(pipeline.focuses)))
	}
	if finding.Path == "" {
		return errors.New("path is required")
	}
	if finding.Side != SideLeft && finding.Side != SideRight {
		return fmt.Errorf("side must be %s or %s", SideLeft, SideRight)
	}
	if finding.Line < 1 {
		return errors.New("line must be positive")
	}
	if pipeline.anchors == nil || !pipeline.anchors.ValidAnchor(finding.Path, finding.Side, finding.Line) {
		return fmt.Errorf("%s:%d on %s is not a changed diff line", finding.Path, finding.Line, finding.Side)
	}
	if !slices.Contains([]Severity{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}, finding.Severity) {
		return fmt.Errorf("unsupported severity %q", finding.Severity)
	}
	if finding.Confidence < 0 || finding.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if finding.ConfidenceRationale == "" || len(finding.ConfidenceRationale) > maxFindingSectionLength {
		return fmt.Errorf("confidence rationale must contain 1-%d characters", maxFindingSectionLength)
	}
	if !slices.Contains([]string{
		"latency",
		"throughput",
		"cpu",
		"memory",
		"gc",
		"io",
		"ipc",
		"subprocess",
		"rendering",
		"layout",
		"startup",
		"network",
	}, finding.PerformanceCategory) {
		return fmt.Errorf("unsupported performance category %q", finding.PerformanceCategory)
	}
	if !slices.Contains([]string{"introduced", "materially-amplified", "pre-existing-critical"}, finding.ChangeCausality) {
		return fmt.Errorf("unsupported change causality %q", finding.ChangeCausality)
	}
	if finding.ChangeCausality == "pre-existing-critical" && finding.Severity != SeverityCritical {
		return errors.New("pre-existing issues may be reported only at critical severity")
	}
	if finding.Title == "" || len(finding.Title) > maxTitleLength {
		return fmt.Errorf("title must contain 1-%d characters", maxTitleLength)
	}
	for name, value := range map[string]string{
		"performance resource": finding.PerformanceResource,
		"performance scaling":  finding.PerformanceScaling,
		"performance outcome":  finding.PerformanceOutcome,
		"previous behavior":    finding.PreviousBehavior,
		"changed behavior":     finding.ChangedBehavior,
		"causal diff evidence": finding.CausalDiffEvidence,
		"impact":               finding.Impact,
		"evidence":             finding.Evidence,
		"recommendation":       finding.Recommendation,
	} {
		if value == "" || len(value) > maxFindingSectionLength {
			return fmt.Errorf("%s must contain 1-%d characters", name, maxFindingSectionLength)
		}
	}
	return nil
}

func normalizeFinding(finding Finding) Finding {
	finding.Focus = strings.TrimSpace(finding.Focus)
	finding.Path = strings.TrimSpace(strings.TrimPrefix(finding.Path, "./"))
	finding.Title = strings.TrimSpace(finding.Title)
	finding.ConfidenceRationale = strings.TrimSpace(finding.ConfidenceRationale)
	finding.PerformanceCategory = strings.ToLower(strings.TrimSpace(finding.PerformanceCategory))
	finding.PerformanceResource = strings.TrimSpace(finding.PerformanceResource)
	finding.PerformanceScaling = strings.TrimSpace(finding.PerformanceScaling)
	finding.PerformanceOutcome = strings.TrimSpace(finding.PerformanceOutcome)
	finding.ChangeCausality = strings.ToLower(strings.TrimSpace(finding.ChangeCausality))
	finding.PreviousBehavior = strings.TrimSpace(finding.PreviousBehavior)
	finding.ChangedBehavior = strings.TrimSpace(finding.ChangedBehavior)
	finding.CausalDiffEvidence = strings.TrimSpace(finding.CausalDiffEvidence)
	finding.Impact = strings.TrimSpace(finding.Impact)
	finding.Evidence = strings.TrimSpace(finding.Evidence)
	finding.Recommendation = strings.TrimSpace(finding.Recommendation)
	return finding
}

func compareFindings(left, right Finding) int {
	if left.Confidence != right.Confidence {
		if left.Confidence > right.Confidence {
			return -1
		}
		return 1
	}
	if severityRank(left.Severity) != severityRank(right.Severity) {
		return severityRank(right.Severity) - severityRank(left.Severity)
	}
	if pathCompare := strings.Compare(left.Path, right.Path); pathCompare != 0 {
		return pathCompare
	}
	return left.Line - right.Line
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}
