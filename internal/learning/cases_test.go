package learning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type fakeResolver struct {
	pulls []review.PullRequest
	err   error
}

func (resolver fakeResolver) ListPullRequestsForCommit(context.Context, string, string, string) ([]review.PullRequest, error) {
	return resolver.pulls, resolver.err
}

func TestEvaluateResolvesIntroducerAndClassifiesCoverage(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	introducerMerged := now.Add(-48 * time.Hour)
	fixer := review.Result{
		PullRequest: review.PullRequest{
			Number: 20, Title: "Fix repeated work", HeadSHA: "fix-head",
			CreatedAt: now.Add(-time.Hour),
		},
		RegressionFixes: []review.RegressionFix{{
			FixedPath: "src/file.ts", BasePath: "src/file.ts",
			IntroducingPath:     "src/file.ts",
			IntroducingCommit:   "1111111111111111111111111111111111111111",
			MechanismFamily:     review.RegressionRepeatedWork,
			PerformanceCategory: "cpu", Mechanism: "loop", Symptom: "jank",
			FixedBehavior: "batch", Evidence: "changed loop", Confidence: 0.99,
		}},
	}
	introducer := review.PullRequest{
		Number: 10, Title: "Add feature", URL: "https://example.test/10",
		State: "closed", AuthorAssociation: "MEMBER", HeadSHA: "intro-head",
		CreatedAt: now.Add(-72 * time.Hour), MergedAt: &introducerMerged,
	}
	result, err := Evaluate(
		context.Background(), "microsoft", "vscode", fixer, 1,
		fakeResolver{pulls: []review.PullRequest{introducer}},
		func(context.Context, review.PullRequest) (review.Result, error) {
			return review.Result{Findings: []review.Finding{{
				ID: "PERF-DETECTED", Path: "src/file.ts", PerformanceCategory: "cpu",
				MechanismFamily: review.RegressionRepeatedWork,
			}}}, nil
		},
		nil, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cases) != 1 || result.Cases[0].Introducer.Number != 10 ||
		result.Cases[0].Coverage.Status != "detected" ||
		len(result.Replays) != 1 || len(result.Unresolved) != 0 {
		t.Fatalf("evaluation = %+v", result)
	}
}

func TestEvaluateRecordsInstructionGapAndUnresolvedCommit(t *testing.T) {
	now := time.Now()
	fixer := review.Result{
		PullRequest: review.PullRequest{Number: 20, CreatedAt: now},
		RegressionFixes: []review.RegressionFix{{
			BasePath: "src/file.ts", IntroducingCommit: "1111111111111111111111111111111111111111",
			IntroducingPath: "src/file.ts",
			MechanismFamily: review.RegressionEagerWork, PerformanceCategory: "startup",
		}},
	}

	merged := now.Add(-time.Hour)
	introducer := review.PullRequest{
		Number: 10, AuthorAssociation: "MEMBER", CreatedAt: now.Add(-2 * time.Hour),
		MergedAt: &merged,
	}
	result, err := Evaluate(
		context.Background(), "microsoft", "vscode", fixer, 1,
		fakeResolver{pulls: []review.PullRequest{introducer}},
		func(context.Context, review.PullRequest) (review.Result, error) {
			return review.Result{}, nil
		},
		nil, now,
	)
	if err != nil || len(result.Cases) != 1 || result.Cases[0].Coverage.Status != "instruction-gap" {
		t.Fatalf("evaluation=%+v err=%v", result, err)
	}

	result, err = Evaluate(
		context.Background(), "microsoft", "vscode", fixer, 1,
		fakeResolver{},
		func(context.Context, review.PullRequest) (review.Result, error) {
			return review.Result{}, errors.New("should not run")
		},
		nil, now,
	)
	if err != nil || len(result.Unresolved) != 1 || len(result.Cases) != 0 {
		t.Fatalf("unresolved evaluation=%+v err=%v", result, err)
	}
}

func TestEvaluateReusesReplayForMultipleCasesFromSameIntroducer(t *testing.T) {
	now := time.Now()
	merged := now.Add(-time.Hour)
	fixer := review.Result{
		PullRequest: review.PullRequest{Number: 20, CreatedAt: now},
		RegressionFixes: []review.RegressionFix{
			{
				FixedPath: "src/a.ts", BasePath: "src/a.ts",
				IntroducingPath:   "src/a.ts",
				IntroducingCommit: "1111111111111111111111111111111111111111",
				MechanismFamily:   review.RegressionRepeatedWork, PerformanceCategory: "cpu",
			},
			{
				FixedPath: "src/b.ts", BasePath: "src/b.ts",
				IntroducingPath:   "src/b.ts",
				IntroducingCommit: "1111111111111111111111111111111111111111",
				MechanismFamily:   review.RegressionEagerWork, PerformanceCategory: "startup",
			},
		},
	}

	introducer := review.PullRequest{
		Number: 10, AuthorAssociation: "MEMBER", CreatedAt: now.Add(-2 * time.Hour),
		MergedAt: &merged, HeadSHA: "intro-head",
	}
	calls := 0
	result, err := Evaluate(
		context.Background(), "microsoft", "vscode", fixer, 2,
		fakeResolver{pulls: []review.PullRequest{introducer}},
		func(context.Context, review.PullRequest) (review.Result, error) {
			calls++
			return review.Result{}, nil
		},
		nil, now,
	)
	if err != nil || len(result.Cases) != 2 || len(result.Replays) != 1 || calls != 1 {
		t.Fatalf("evaluation=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestEvaluateRejectsPullRequestMergedAfterFixerOpened(t *testing.T) {
	now := time.Now()
	mergedAfterFixer := now.Add(time.Hour)
	fixer := review.Result{
		PullRequest: review.PullRequest{Number: 20, CreatedAt: now},
		RegressionFixes: []review.RegressionFix{{
			IntroducingCommit: "1111111111111111111111111111111111111111",
			IntroducingPath:   "src/file.ts",
			MechanismFamily:   review.RegressionRepeatedWork,
		}},
	}

	result, err := Evaluate(
		context.Background(), "microsoft", "vscode", fixer, 1,
		fakeResolver{pulls: []review.PullRequest{{
			Number: 10, AuthorAssociation: "MEMBER",
			CreatedAt: now.Add(-time.Hour), MergedAt: &mergedAfterFixer,
		}}},
		func(context.Context, review.PullRequest) (review.Result, error) {
			t.Fatal("unexpected replay")
			return review.Result{}, nil
		},
		nil, now,
	)
	if err != nil || len(result.Unresolved) != 1 {
		t.Fatalf("evaluation=%+v err=%v", result, err)
	}
}

func TestCaseIDIncludesCompleteFixedCoordinate(t *testing.T) {
	base := RegressionCase{
		Fixer:               PullRequestReference{HeadSHA: "fixer"},
		Introducer:          PullRequestReference{HeadSHA: "introducer"},
		IntroducingCommit:   "1111111111111111111111111111111111111111",
		MechanismFamily:     review.RegressionRepeatedWork,
		PerformanceCategory: "cpu",
		BasePath:            "src/file.ts",
		IntroducingPath:     "src/file.ts",
		FixedPath:           "src/fix.ts",
		FixedSide:           review.SideRight,
		BaseLine:            10,
		FixedLine:           11,
	}
	baseID := caseID(base)
	changedPath := base
	changedPath.FixedPath = "src/other.ts"
	changedSide := base
	changedSide.FixedSide = review.SideLeft
	if caseID(changedPath) == baseID || caseID(changedSide) == baseID {
		t.Fatalf("case ID did not include complete fixed coordinate")
	}
}
