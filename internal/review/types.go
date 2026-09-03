package review

import (
	"slices"
	"strings"
	"time"
)

type PullRequest struct {
	Number            int
	Title             string
	Body              string
	URL               string
	State             string
	Draft             bool
	AuthorLogin       string
	AuthorAssociation string
	BaseRef           string
	BaseSHA           string
	HeadRef           string
	HeadSHA           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	MergedAt          *time.Time
	Labels            []string
}

func (pull PullRequest) IsTeamAuthored() bool {
	return pull.AuthorAssociation == "MEMBER" || pull.AuthorAssociation == "OWNER"
}

func (pull PullRequest) HasAnyLabel(labels []string) bool {
	for _, configured := range labels {
		if slices.ContainsFunc(pull.Labels, func(label string) bool {
			return strings.EqualFold(label, configured)
		}) {
			return true
		}
	}
	return false
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
	ID                  string
	Focus               string
	Path                string
	Side                Side
	Line                int
	Severity            Severity
	Confidence          float64
	ConfidenceRationale string
	PerformanceCategory string
	MechanismFamily     RegressionMechanismFamily `json:"MechanismFamily,omitempty"`
	PerformanceResource string
	PerformanceScaling  string
	PerformanceOutcome  string
	ChangeCausality     string
	PreviousBehavior    string
	ChangedBehavior     string
	CausalDiffEvidence  string
	Title               string
	Impact              string
	Evidence            string
	Recommendation      string
}

type Result struct {
	PullRequest     PullRequest
	Findings        []Finding
	RegressionFixes []RegressionFix
	Analysis        Analysis
	Stats           Stats
}

type RegressionMechanismFamily string

const (
	RegressionRepeatedWork       RegressionMechanismFamily = "repeated-work"
	RegressionUnboundedRetention RegressionMechanismFamily = "unbounded-retention"
	RegressionEagerWork          RegressionMechanismFamily = "eager-work"
	RegressionBoundaryFanout     RegressionMechanismFamily = "boundary-fanout"
	RegressionMissingCoalescing  RegressionMechanismFamily = "missing-coalescing"
	RegressionSynchronousUI      RegressionMechanismFamily = "synchronous-ui-work"
	RegressionCacheLifecycle     RegressionMechanismFamily = "cache-lifecycle"
	RegressionCleanupLifecycle   RegressionMechanismFamily = "cleanup-lifecycle"
	RegressionSerialization      RegressionMechanismFamily = "serialization-allocation"
	RegressionConcurrencyBurst   RegressionMechanismFamily = "concurrency-burst"
)

type RegressionFix struct {
	FixedPath           string
	FixedSide           Side
	FixedLine           int
	BasePath            string
	BaseLine            int
	IntroducingPath     string
	IntroducingCommit   string
	MechanismFamily     RegressionMechanismFamily
	PerformanceCategory string
	Mechanism           string
	Symptom             string
	FixedBehavior       string
	Evidence            string
	Confidence          float64
}

type Analysis struct {
	RelevantIssueFamilies []string
	Scenarios             []ScenarioAnalysis
	Summary               string
}

type ScenarioAnalysis struct {
	Scenario             string
	CriticalPath         string
	ExpensiveBoundaries  []BoundaryAnalysis
	ScalingInput         string
	EffectiveConcurrency string
	CacheBehavior        string
	MechanismConfidence  string
	MagnitudeUncertainty string
	Verdict              string
}

type BoundaryAnalysis struct {
	Kind               string
	Location           string
	Operation          string
	Cardinality        string
	IntroducedByDiff   bool
	PreviousBehavior   string
	CriticalPathEffect string
}

type Stats struct {
	Model                   string
	ActualModels            []string
	ReasoningEffort         string
	ActualReasoningEfforts  []string
	APIEndpoints            []string
	StartedAt               time.Time
	CompletedAt             time.Time
	WallClockMilliseconds   int64
	ModelCalls              int64
	InputTokens             int64
	OutputTokens            int64
	TotalTokens             int64
	ReasoningTokens         int64
	CacheReadTokens         int64
	CacheWriteTokens        int64
	APIDurationMilliseconds int64
	ToolCalls               int64
	NanoAIUnits             float64
	ModelBillingMultipliers []float64
	BillingTokensByType     map[string]int64
	USDollars               *float64
	CostNote                string
}
