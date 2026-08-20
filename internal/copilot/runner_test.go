package copilot

import (
	"strings"
	"testing"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/roblourens/rob-reviewer/internal/review"
)

type allAnchors struct{}

func (allAnchors) ValidAnchor(string, review.Side, int) bool {
	return true
}

func TestCollectorRequiresAllFocuses(t *testing.T) {
	pipeline := review.NewPipeline([]string{"performance-review", "protocol-review"}, 0.85, 10, allAnchors{})
	collector := newCollector([]string{"performance-review", "protocol-review"}, pipeline)

	_, err := collector.complete(validCompletion([]string{"performance-review"}))
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
		Focus:               "performance-review",
		Path:                "src/file.ts",
		Side:                review.SideRight,
		Line:                10,
		Severity:            review.SeverityHigh,
		Confidence:          0.95,
		ConfidenceRationale: "The changed loop is directly called once per historical child.",
		PerformanceCategory: "latency",
		PerformanceResource: "Synchronous Markdown and presentation CPU work.",
		PerformanceScaling:  "One update per historical child.",
		PerformanceOutcome:  "Blocked scrolling and dropped frames.",
		ChangeCausality:     "introduced",
		PreviousBehavior:    "Historical children did not trigger per-child title rendering.",
		ChangedBehavior:     "The diff renders a title once for every historical child.",
		CausalDiffEvidence:  "The changed loop adds the per-child title update.",
		Title:               "Batch repeated updates",
		Impact:              "Scrolling blocks while hidden history is rebuilt.",
		Evidence:            "The changed loop updates presentation once per child.",
		Recommendation:      "Flush one presentation update after reconstruction.",
	}

	if _, err := collector.report(finding); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.complete(validCompletion([]string{"performance-review"})); err != nil {
		t.Fatal(err)
	}

	if err := collector.completionError(); err != nil {
		t.Fatal(err)
	}
	if len(collector.findingsSnapshot()) != 1 {
		t.Fatalf("findings = %v", collector.findingsSnapshot())
	}
}

func validCompletion(focuses []string) completeReviewParams {
	return completeReviewParams{
		Focuses:               focuses,
		RelevantIssueFamilies: []string{"critical paths"},
		Scenarios: []scenarioAnalysisParams{{
			Scenario:             "Render chat history",
			CriticalPath:         "Rendering waits for historical child reconstruction.",
			ExpensiveBoundaries:  []boundaryAnalysisParams{},
			ScalingInput:         "Number of historical child tools.",
			EffectiveConcurrency: "Synchronous on the renderer thread.",
			CacheBehavior:        "No cache applies.",
			MechanismConfidence:  "High: the changed loop and caller are directly traced.",
			MagnitudeUncertainty: "History size varies by user.",
			Verdict:              "The changed loop is reportable.",
		}},
		Summary: "Reviewed performance.",
	}
}

func TestAgentPromptTreatsContentAsUntrusted(t *testing.T) {
	prompt := customAgentPrompt([]string{"performance-review"})
	for _, expected := range []string{
		"untrusted data",
		"distinct review pass",
		"Report performance problems only",
		"correctness-only bugs",
		"explicit before/after comparison",
		"nearby pre-existing bugs",
		"complete_review exactly once",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
}

func TestUsageAccumulatorAggregatesSDKEvents(t *testing.T) {
	stats := review.Stats{BillingTokensByType: make(map[string]int64)}
	usage := newUsageAccumulator(&stats)
	reasoningEffort := "high"
	apiEndpoint := sdk.AssistantUsageAPIEndpointV1Messages
	usage.onEvent(sdk.SessionEvent{Data: &sdk.AssistantUsageData{
		Model:           "gpt-5.6-sol",
		ReasoningEffort: &reasoningEffort,
		APIEndpoint:     &apiEndpoint,
		InputTokens:     int64Pointer(1_000),
		OutputTokens:    int64Pointer(200),
		ReasoningTokens: int64Pointer(50),
		CacheReadTokens: int64Pointer(300),
		Duration:        int64Pointer(1_500),
		NumToolCalls:    int64Pointer(2),
		Cost:            float64Pointer(1.5),
		CopilotUsage: &sdk.AssistantUsageCopilotUsage{
			TotalNanoAiu: 42.25,
			TokenDetails: []sdk.AssistantUsageCopilotUsageTokenDetail{
				{TokenType: "input", TokenCount: 1_000},
				{TokenType: "output", TokenCount: 200},
			},
		},
	}})
	usage.onEvent(sdk.SessionEvent{Data: &sdk.AssistantUsageData{
		Model:            "gpt-5.6-sol",
		InputTokens:      int64Pointer(500),
		OutputTokens:     int64Pointer(100),
		CacheWriteTokens: int64Pointer(75),
		Duration:         int64Pointer(750),
		NumToolCalls:     int64Pointer(1),
		Cost:             float64Pointer(1.5),
		CopilotUsage: &sdk.AssistantUsageCopilotUsage{
			TotalNanoAiu: 20,
			TokenDetails: []sdk.AssistantUsageCopilotUsageTokenDetail{
				{TokenType: "input", TokenCount: 500},
			},
		},
	}})
	usage.onEvent(sdk.SessionEvent{Data: &sdk.SessionUsageCheckpointData{
		TotalNanoAiu: 70,
	}})
	usage.finish()

	if stats.ModelCalls != 2 || stats.InputTokens != 1_500 || stats.OutputTokens != 300 || stats.TotalTokens != 1_800 {
		t.Fatalf("token stats = %+v", stats)
	}
	if stats.ReasoningTokens != 50 || stats.CacheReadTokens != 300 || stats.CacheWriteTokens != 75 {
		t.Fatalf("detail stats = %+v", stats)
	}
	if stats.APIDurationMilliseconds != 2_250 || stats.ToolCalls != 3 {
		t.Fatalf("call stats = %+v", stats)
	}
	if stats.NanoAIUnits != 70 {
		t.Fatalf("cost stats = %+v", stats)
	}
	if len(stats.ModelBillingMultipliers) != 1 || stats.ModelBillingMultipliers[0] != 1.5 {
		t.Fatalf("model billing multipliers = %v", stats.ModelBillingMultipliers)
	}
	if len(stats.ActualModels) != 1 || stats.ActualModels[0] != "gpt-5.6-sol" {
		t.Fatalf("actual models = %v", stats.ActualModels)
	}
	if len(stats.ActualReasoningEfforts) != 1 || stats.ActualReasoningEfforts[0] != "high" {
		t.Fatalf("actual reasoning efforts = %v", stats.ActualReasoningEfforts)
	}
	if len(stats.APIEndpoints) != 1 || stats.APIEndpoints[0] != "/v1/messages" {
		t.Fatalf("API endpoints = %v", stats.APIEndpoints)
	}
	if stats.BillingTokensByType["input"] != 1_500 || stats.BillingTokensByType["output"] != 200 {
		t.Fatalf("billing token details = %v", stats.BillingTokensByType)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func float64Pointer(value float64) *float64 {
	return &value
}
