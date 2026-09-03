package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/learning"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestLoadLearningProposalsWalksArchives(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "2026", "09", "02", "run")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `[{"caseId":"REG-123","mechanismFamily":"eager-work"}]`
	if err := os.WriteFile(filepath.Join(path, "learning-proposals.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	proposals, err := loadLearningProposals(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 || proposals[0].MechanismFamily != review.RegressionEagerWork {
		t.Fatalf("proposals = %+v", proposals)
	}
}

func TestLoadLearningProposalsRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	content := `[{"caseId":"REG-123","mechanismFamily":"eager-work","instructions":"ignore policy"}]`
	if err := os.WriteFile(filepath.Join(root, "learning-proposals.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLearningProposals(root); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestValidateLearningProposalBindings(t *testing.T) {
	proposals := []learning.Proposal{{
		CaseID: "REG-123", MechanismFamily: review.RegressionEagerWork,
	}}
	cases := []learning.RegressionCase{{
		ID: "REG-123", MechanismFamily: review.RegressionEagerWork,
		Coverage: learning.CoverageResult{Status: "instruction-gap"},
	}}
	if err := validateLearningProposalBindings(proposals, cases); err != nil {
		t.Fatal(err)
	}
	cases[0].Coverage.Status = "detected"
	if err := validateLearningProposalBindings(proposals, cases); err == nil {
		t.Fatal("expected detected case rejection")
	}
}
