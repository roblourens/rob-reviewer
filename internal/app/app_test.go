package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/review"
	"github.com/roblourens/rob-reviewer/internal/state"
)

func TestPollIsNoOpWhenAutomationDisabled(t *testing.T) {
	app := &App{
		config: config.Config{
			Automation: config.AutomationConfig{Enabled: false},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	result, err := app.Poll(context.Background(), "", "")
	if err != nil || len(result.Reviews) != 0 || len(result.Poll.Reviewed) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestReviewPullRequestSkipsWhenEarlierHeadWasReviewed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/microsoft/vscode/pulls/7":
			io.WriteString(writer, `{
				"number":7,
				"state":"open",
				"draft":false,
				"author_association":"MEMBER",
				"user":{"login":"member"},
				"base":{"ref":"main","sha":"base"},
				"head":{"ref":"feature","sha":"new-head"}
			}`)
		case "/user":
			io.WriteString(writer, `{"login":"review-bot"}`)
		case "/repos/microsoft/vscode/pulls/7/reviews":
			io.WriteString(writer, `[{
				"id":42,
				"body":"<!-- rob-reviewer:v1 pr=7 head=old-head -->",
				"state":"COMMENTED",
				"user":{"login":"review-bot"}
			}]`)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	app := &App{
		config: config.Config{
			Target: config.TargetConfig{Owner: "microsoft", Repo: "vscode"},
		},
		reviewClient: github.NewClientWithBaseURL(server.Client(), "token", server.URL),
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	result, analyzed, err := app.ReviewPullRequest(context.Background(), 7, true)
	if err != nil || analyzed || result.PullRequest.HeadSHA != "new-head" {
		t.Fatalf("analyzed=%v result=%+v err=%v", analyzed, result, err)
	}
}

func TestPollPreservesHumanApprovedPendingReview(t *testing.T) {
	var saved state.State
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/owner/reviewer/contents/state.json":
			if request.Method == http.MethodPut {
				var update struct {
					Content []byte `json:"content"`
				}
				if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(update.Content, &saved); err != nil {
					t.Fatal(err)
				}
				io.WriteString(writer, `{}`)
				return
			}
			content, err := json.Marshal(state.State{Version: state.CurrentVersion, Initialized: true})
			if err != nil {
				t.Fatal(err)
			}
			json.NewEncoder(writer).Encode(map[string]string{
				"sha": "state-sha", "encoding": "base64", "content": base64.StdEncoding.EncodeToString(content),
			})
		case "/repos/microsoft/vscode/pulls", "/repos/microsoft/vscode/pulls/7":
			pull := map[string]any{
				"number": 7, "state": "open", "author_association": "MEMBER", "updated_at": time.Now().UTC(),
				"head": map[string]string{"sha": "head"}, "base": map[string]string{"sha": "base"},
			}
			if request.URL.Path == "/repos/microsoft/vscode/pulls" {
				json.NewEncoder(writer).Encode([]any{pull})
			} else {
				json.NewEncoder(writer).Encode(pull)
			}
		case "/user":
			io.WriteString(writer, `{"login":"review-bot"}`)
		case "/repos/microsoft/vscode/pulls/7/reviews":
			io.WriteString(writer, `[{
				"id":42,
				"body":"Human-approved experimental performance review.\n<!-- rob-reviewer:v1 pr=7 head=head -->",
				"state":"PENDING",
				"user":{"login":"review-bot"}
			}]`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	client := github.NewClientWithBaseURL(server.Client(), "token", server.URL)
	app := &App{
		config: config.Config{
			Target:      config.TargetConfig{Owner: "microsoft", Repo: "vscode"},
			Poll:        config.PollConfig{MaxPerRun: 5, MaxPerDay: 25, ScanWindowHours: 168, MaxPendingHours: 24},
			Automation:  config.AutomationConfig{Enabled: true},
			Publication: config.PublicationConfig{Mode: "automatic"},
			State:       config.StateConfig{Branch: "reviewer-state", Path: "state.json"},
		},
		reviewClient: client, stateClient: client, hasStateToken: true,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	result, err := app.Poll(context.Background(), "owner/reviewer", t.TempDir())
	if err != nil || len(result.Poll.Published) != 0 || !saved.PullRequests[7].Reviewed {
		t.Fatalf("result=%+v saved=%+v err=%v", result, saved, err)
	}
}

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
			ID:                  "PERF-1234567890AB",
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
	for _, expected := range []string{"&#64;team", "&lt;script&gt;", "\\!\\[image\\]", "PERF-1234567890AB", "Review statistics"} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatted Markdown missing %q: %s", expected, formatted)
		}
	}
}
