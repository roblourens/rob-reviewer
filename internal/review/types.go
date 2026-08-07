package review

import "time"

type PullRequest struct {
	Number    int
	Title     string
	Body      string
	URL       string
	State     string
	Draft     bool
	BaseRef   string
	BaseSHA   string
	HeadRef   string
	HeadSHA   string
	CreatedAt time.Time
}

type Side string

const (
	SideLeft  Side = "LEFT"
	SideRight Side = "RIGHT"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Finding struct {
	Focus          string
	Path           string
	Side           Side
	Line           int
	Severity       Severity
	Confidence     float64
	Title          string
	Impact         string
	Evidence       string
	Recommendation string
}

type Result struct {
	PullRequest PullRequest
	Findings    []Finding
}
