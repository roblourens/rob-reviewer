package review

import (
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
	if finding.Title == "" || len(finding.Title) > maxTitleLength {
		return fmt.Errorf("title must contain 1-%d characters", maxTitleLength)
	}
	for name, value := range map[string]string{
		"impact":         finding.Impact,
		"evidence":       finding.Evidence,
		"recommendation": finding.Recommendation,
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
