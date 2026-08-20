package app

import (
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestParseRepository(t *testing.T) {
	owner, repo, err := parseRepository("owner/repo")
	if err != nil || owner != "owner" || repo != "repo" {
		t.Fatalf("owner = %q, repo = %q, err = %v", owner, repo, err)
	}
	if _, _, err := parseRepository("invalid"); err == nil {
		t.Fatal("expected invalid repository error")
	}
}

func TestParsePullRequestNumber(t *testing.T) {
	number, err := ParsePullRequestNumber("42")
	if err != nil || number != 42 {
		t.Fatalf("number = %d, err = %v", number, err)
	}
	if _, err := ParsePullRequestNumber("0"); err == nil {
		t.Fatal("expected invalid pull request number error")
	}
}

func TestFormatResultMarkdownSanitizesUntrustedContent(t *testing.T) {
	result := review.Result{
		PullRequest: review.PullRequest{
			Number:  7,
			Title:   "@team <script>\nheading",
			URL:     "https://github.com/microsoft/vscode/pull/7",
			HeadSHA: "abc",
		},
		Findings: []review.Finding{{
			Path:                "src/`file`.ts",
			Side:                review.SideRight,
			Line:                12,
			Severity:            review.SeverityHigh,
			Confidence:          0.95,
			ConfidenceRationale: "@team traced <details>",
			PerformanceCategory: "latency",
			PerformanceResource: "<img src=x>",
			PerformanceScaling:  "**per item**",
			PerformanceOutcome:  "@team delay",
			ChangeCausality:     "introduced",
			PreviousBehavior:    "<details>before</details>",
			ChangedBehavior:     "![after](url)",
			CausalDiffEvidence:  "@team changed line",
			Title:               "Avoid ![image](url)",
			Impact:              "<img src=x>",
			Evidence:            "**bold**",
			Recommendation:      "`code`",
		}},
		Stats: review.Stats{
			Model:           "gpt-5.6-sol",
			ReasoningEffort: "high",
		},
	}

	formatted := FormatResultMarkdown(result)
	for _, forbidden := range []string{"@team", "<script>", "<img", "![image]", "**bold**"} {
		if strings.Contains(formatted, forbidden) {
			t.Fatalf("formatted Markdown contains %q: %s", forbidden, formatted)
		}
	}
	for _, expected := range []string{"&#64;team", "&lt;script&gt;", "\\!\\[image\\]", "Review statistics"} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatted Markdown missing %q: %s", expected, formatted)
		}
	}
}
