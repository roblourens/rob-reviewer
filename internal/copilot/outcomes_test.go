package copilot

import (
	"testing"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/roblourens/rob-reviewer/internal/outcomes"
)

func TestFeedbackCollectorRequiresValidEvidenceAndSingleCompletion(t *testing.T) {
	collector := &assessmentCollector{input: outcomes.Input{Author: "owner", Comments: []outcomes.Comment{
		{ID: 1, Author: "reviewer", Body: "Repeated work."},
		{ID: 2, Author: "owner", Body: "Thanks, fixed."},
	}}}
	if _, err := collector.result(); err == nil {
		t.Fatal("missing submission accepted")
	}
	assessment := outcomes.Assessment{Feedback: "accepted", Confidence: .9, Rationale: "Author claims a fix.",
		Evidence: []outcomes.Evidence{{CommentID: 2, Quote: "invented"}}}
	if _, err := collector.submit(assessment, sdk.ToolInvocation{}); err == nil {
		t.Fatal("invented evidence accepted")
	}
	assessment.Evidence[0].Quote = "Thanks, fixed."
	if _, err := collector.submit(assessment, sdk.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.submit(assessment, sdk.ToolInvocation{}); err == nil {
		t.Fatal("duplicate assessment accepted")
	}
	if result, err := collector.result(); err != nil || result.Feedback != "accepted" {
		t.Fatalf("result %+v err %v", result, err)
	}
}

func TestFeedbackSessionHasNoAmbientCapabilities(t *testing.T) {
	runner := &Runner{baseDirectory: "/isolated", options: Options{Model: "model", ReasoningEffort: "high"}}
	cfg := feedbackSessionConfig(runner, &assessmentCollector{})
	for _, enabled := range []*bool{
		cfg.EnableConfigDiscovery, cfg.EnableOnDemandInstructionDiscovery, cfg.EnableFileHooks,
		cfg.EnableHostGitOperations, cfg.EnableSessionStore, cfg.EnableSkills,
	} {
		if enabled == nil || *enabled {
			t.Fatal("ambient capability enabled")
		}
	}
	if !*cfg.SkipCustomInstructions || !*cfg.CustomAgentsLocalOnly || cfg.WorkingDirectory != "/isolated" ||
		len(cfg.AvailableTools) != 1 || cfg.AvailableTools[0] != "custom:report_author_feedback" || len(cfg.Tools) != 1 ||
		len(cfg.CustomAgents) != 0 || len(cfg.SkillDirectories) != 0 || cfg.SystemMessage.Content != feedbackInstructions {
		t.Fatalf("unsafe feedback config: %+v", cfg)
	}
}
