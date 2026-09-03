package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type PullRequestResolver interface {
	ListPullRequestsForCommit(ctx context.Context, owner, repo, commit string) ([]review.PullRequest, error)
}

type Analyzer func(context.Context, review.PullRequest) (review.Result, error)

type RegressionCase struct {
	ID                       string                           `json:"id"`
	Fixer                    PullRequestReference             `json:"fixer"`
	Introducer               PullRequestReference             `json:"introducer"`
	IntroducingCommit        string                           `json:"introducingCommit"`
	MechanismFamily          review.RegressionMechanismFamily `json:"mechanismFamily"`
	PerformanceCategory      string                           `json:"performanceCategory"`
	FixedPath                string                           `json:"fixedPath"`
	FixedSide                review.Side                      `json:"fixedSide"`
	BasePath                 string                           `json:"basePath"`
	BaseLine                 int                              `json:"baseLine"`
	FixedLine                int                              `json:"fixedLine"`
	IntroducingPath          string                           `json:"introducingPath"`
	Mechanism                string                           `json:"mechanism"`
	Symptom                  string                           `json:"symptom"`
	FixedBehavior            string                           `json:"fixedBehavior"`
	Evidence                 string                           `json:"evidence"`
	Confidence               float64                          `json:"confidence"`
	Coverage                 CoverageResult                   `json:"coverage"`
	IntroducerJSONReport     string                           `json:"introducerJsonReport"`
	IntroducerMarkdownReport string                           `json:"introducerMarkdownReport"`
	RunID                    string                           `json:"runId,omitempty"`
	RunAttempt               int                              `json:"runAttempt,omitempty"`
	RunArchivePath           string                           `json:"runArchivePath,omitempty"`
	CreatedAt                time.Time                        `json:"createdAt"`
}

type PullRequestReference struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	BaseSHA string `json:"baseSha"`
	HeadSHA string `json:"headSha"`
}

type CoverageResult struct {
	Status             string   `json:"status"`
	DetectedFindingIDs []string `json:"detectedFindingIds,omitempty"`
}

type UnresolvedRegressionFix struct {
	Fixer             PullRequestReference             `json:"fixer"`
	IntroducingCommit string                           `json:"introducingCommit"`
	MechanismFamily   review.RegressionMechanismFamily `json:"mechanismFamily"`
	Reason            string                           `json:"reason"`
}

type Evaluation struct {
	Cases      []RegressionCase          `json:"cases"`
	Unresolved []UnresolvedRegressionFix `json:"unresolved"`
	Replays    []review.Result           `json:"-"`
}

type ReplayCache map[string]review.Result

func Evaluate(
	ctx context.Context,
	owner, repo string,
	fixer review.Result,
	maxCases int,
	resolver PullRequestResolver,
	analyze Analyzer,
	replayCache ReplayCache,
	now time.Time,
) (Evaluation, error) {
	var result Evaluation
	if replayCache == nil {
		replayCache = make(ReplayCache)
	}
	for _, fix := range fixer.RegressionFixes {
		if len(result.Cases) >= maxCases {
			break
		}
		pulls, err := resolver.ListPullRequestsForCommit(ctx, owner, repo, fix.IntroducingCommit)
		if err != nil {
			return result, fmt.Errorf("resolve introducing commit %s: %w", fix.IntroducingCommit, err)
		}
		introducer, ok := selectIntroducer(fixer.PullRequest, pulls)
		if !ok {
			result.Unresolved = append(result.Unresolved, UnresolvedRegressionFix{
				Fixer:             pullReference(fixer.PullRequest),
				IntroducingCommit: fix.IntroducingCommit,
				MechanismFamily:   fix.MechanismFamily,
				Reason:            "no earlier merged team pull request is associated with the blamed commit",
			})
			continue
		}
		replayKey := fmt.Sprintf("%d\x00%s", introducer.Number, introducer.HeadSHA)
		replay, replayed := replayCache[replayKey]
		if !replayed {
			replay, err = analyze(ctx, introducer)
			if err != nil {
				return result, fmt.Errorf("replay introducing PR %d: %w", introducer.Number, err)
			}
			replayCache[replayKey] = replay
			result.Replays = append(result.Replays, replay)
		}
		coverage := CoverageResult{Status: "instruction-gap"}
		for _, finding := range replay.Findings {
			if finding.PerformanceCategory != fix.PerformanceCategory {
				continue
			}
			if finding.MechanismFamily != fix.MechanismFamily {
				continue
			}
			if finding.Path != fix.IntroducingPath {
				continue
			}
			coverage.Status = "detected"
			coverage.DetectedFindingIDs = append(coverage.DetectedFindingIDs, finding.ID)
		}
		slices.Sort(coverage.DetectedFindingIDs)
		caseRecord := RegressionCase{
			Fixer:               pullReference(fixer.PullRequest),
			Introducer:          pullReference(introducer),
			IntroducingCommit:   fix.IntroducingCommit,
			MechanismFamily:     fix.MechanismFamily,
			PerformanceCategory: fix.PerformanceCategory,
			FixedPath:           fix.FixedPath,
			FixedSide:           fix.FixedSide,
			BasePath:            fix.BasePath,
			BaseLine:            fix.BaseLine,
			FixedLine:           fix.FixedLine,
			IntroducingPath:     fix.IntroducingPath,
			Mechanism:           fix.Mechanism,
			Symptom:             fix.Symptom,
			FixedBehavior:       fix.FixedBehavior,
			Evidence:            fix.Evidence,
			Confidence:          fix.Confidence,
			Coverage:            coverage,
			CreatedAt:           now.UTC(),
		}
		stem := fmt.Sprintf("pr-%d-%s", introducer.Number, shortSHA(introducer.HeadSHA))
		caseRecord.IntroducerJSONReport = stem + ".json"
		caseRecord.IntroducerMarkdownReport = stem + ".md"
		caseRecord.ID = caseID(caseRecord)
		result.Cases = append(result.Cases, caseRecord)
	}

	return result, nil
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func selectIntroducer(fixer review.PullRequest, pulls []review.PullRequest) (review.PullRequest, bool) {
	candidates := slices.DeleteFunc(slices.Clone(pulls), func(pull review.PullRequest) bool {
		return pull.Number == fixer.Number || !pull.IsTeamAuthored() || pull.MergedAt == nil ||
			(!fixer.CreatedAt.IsZero() && !pull.MergedAt.Before(fixer.CreatedAt))
	})
	if len(candidates) == 0 {
		return review.PullRequest{}, false
	}
	slices.SortFunc(candidates, func(left, right review.PullRequest) int {
		if compared := right.MergedAt.Compare(*left.MergedAt); compared != 0 {
			return compared
		}
		return right.Number - left.Number
	})
	return candidates[0], true
}

func pullReference(pull review.PullRequest) PullRequestReference {
	return PullRequestReference{
		Number: pull.Number, Title: pull.Title, URL: pull.URL,
		BaseSHA: pull.BaseSHA, HeadSHA: pull.HeadSHA,
	}
}

func caseID(record RegressionCase) string {
	content := strings.Join([]string{
		record.Fixer.HeadSHA,
		record.Introducer.HeadSHA,
		record.IntroducingCommit,
		string(record.MechanismFamily),
		record.PerformanceCategory,
		record.BasePath,
		record.IntroducingPath,
		record.FixedPath,
		string(record.FixedSide),
		fmt.Sprintf("%d", record.BaseLine),
		fmt.Sprintf("%d", record.FixedLine),
	}, "\x00")
	digest := sha256.Sum256([]byte(content))
	return "REG-" + strings.ToUpper(hex.EncodeToString(digest[:6]))
}
