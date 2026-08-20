package review

import "time"

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
}

func (pull PullRequest) IsTeamAuthored() bool {
	return pull.AuthorAssociation == "MEMBER" || pull.AuthorAssociation == "OWNER"
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
	Focus               string
	Path                string
	Side                Side
	Line                int
	Severity            Severity
	Confidence          float64
	ConfidenceRationale string
	PerformanceCategory string
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
	PullRequest PullRequest
	Findings    []Finding
	Analysis    Analysis
	Stats       Stats
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
