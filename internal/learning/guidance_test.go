package learning

import (
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestApplyProposalsUsesPredefinedGuidanceAndIsIdempotent(t *testing.T) {
	proposals := []Proposal{{
		CaseID:          "REG-123",
		MechanismFamily: review.RegressionSerialization,
	}}
	updated, changed, err := ApplyProposals("# Learned\n", proposals)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if !strings.Contains(updated, "<!-- learned-family:serialization-allocation -->") ||
		!strings.Contains(updated, "unbounded data before a downstream cap") ||
		strings.Contains(updated, "REG-123") {
		t.Fatalf("unexpected guidance: %s", updated)
	}
	again, changed, err := ApplyProposals(updated, proposals)
	if err != nil || changed || again != updated {
		t.Fatalf("idempotent changed=%v err=%v", changed, err)
	}
}

func TestProposalsIncludeOnlyInstructionGaps(t *testing.T) {
	proposals := Proposals([]RegressionCase{
		{ID: "REG-GAP", MechanismFamily: review.RegressionEagerWork, Coverage: CoverageResult{Status: "instruction-gap"}},
		{ID: "REG-DETECTED", MechanismFamily: review.RegressionRepeatedWork, Coverage: CoverageResult{Status: "detected"}},
	})
	if len(proposals) != 1 || proposals[0].CaseID != "REG-GAP" {
		t.Fatalf("proposals = %+v", proposals)
	}
}
