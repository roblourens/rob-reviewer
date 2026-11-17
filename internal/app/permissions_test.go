package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
	"gopkg.in/yaml.v3"
)

func TestAnalysisOnlyCannotPublishOrPersistRemoteState(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "reviewers", "performance", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: performance-review\n---\nReview performance.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, "reviewer.yaml")
	if err := os.WriteFile(cfg, []byte(`
target: {owner: owner, repo: repo}
automation: {enabled: true}
publication: {mode: automatic, reviewerLogin: review-bot}
review: {focuses: [performance-review]}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := New(Options{
		ConfigPath: cfg, AnalysisOnly: true,
		ReviewToken: "read-only", StateToken: "read-only", CopilotToken: "inference",
	})
	if err != nil {
		t.Fatal(err)
	}
	if app.config.Publication.Mode != "approval" {
		t.Fatal("automatic configuration enabled analysis publication")
	}
	if _, _, err := app.reviewOne(context.Background(), review.PullRequest{Number: 7}, true); err == nil {
		t.Fatal("analysis accepted publication")
	}
	if _, err := app.Poll(context.Background(), "owner/repo", t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "local state-update") {
		t.Fatalf("analysis accepted remote state persistence: %v", err)
	}
}

func TestWorkflowSeparatesAnalysisFromRepositoryWrites(t *testing.T) {
	content, err := os.ReadFile("../../.github/workflows/review-vscode.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Needs       any               `yaml:"needs"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
				With map[string]any    `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	if workflow.Permissions["contents"] != "read" {
		t.Fatal("workflow grants default write access")
	}
	analysis, exists := workflow.Jobs["review"]
	if !exists || analysis.Permissions["contents"] == "write" {
		t.Fatal("analysis job is missing or has write access")
	}
	analysisCommands := 0
	for _, step := range analysis.Steps {
		if strings.Contains(step.Run, "rob-reviewer poll ") || strings.Contains(step.Run, "rob-reviewer review ") {
			analysisCommands++
			if !strings.Contains(step.Run, "--analysis-only") {
				t.Fatal("analysis command may publish or persist remotely")
			}
		}
		for name, value := range step.Env {
			if name == "REVIEW_GITHUB_TOKEN" || strings.Contains(value, "secrets.REVIEW_GITHUB_TOKEN") {
				t.Fatal("analysis receives a publication credential")
			}
		}
	}
	if analysisCommands != 2 {
		t.Fatalf("analysis command coverage = %d", analysisCommands)
	}
	persist, exists := workflow.Jobs["persist"]
	if !exists || persist.Permissions["contents"] != "write" {
		t.Fatal("state persistence is not isolated after analysis")
	}
	deps, ok := persist.Needs.([]any)
	if !ok || !slices.Contains(deps, any("review")) || !slices.Contains(deps, any("publish")) {
		t.Fatal("persistence does not wait for both analysis and publication")
	}
	for _, step := range persist.Steps {
		for _, value := range step.Env {
			if strings.Contains(value, "secrets.") {
				t.Fatal("persistence receives a user credential")
			}
		}
		if strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With["cache"] != false {
			t.Fatal("privileged persistence shares writable build caches with analysis")
		}
	}
	publish, exists := workflow.Jobs["publish"]
	if !exists || publish.Needs != "review" || publish.Permissions["contents"] != "read" {
		t.Fatal("publisher is not isolated after analysis")
	}
	publishCommands := 0
	for _, step := range publish.Steps {
		for key, value := range step.Env {
			if strings.Contains(value, "COPILOT_GITHUB_TOKEN") || strings.Contains(value, "REVIEW_READ_TOKEN") {
				t.Fatal("publisher receives analysis credentials")
			}
			if key == "REVIEW_GITHUB_TOKEN" {
				publishCommands++
				if step.Run != "go run ./cmd/rob-reviewer publish-run --run-dir review-results" {
					t.Fatal("publication credential is available outside the trusted publisher")
				}
			}
		}
		if strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With["cache"] != false {
			t.Fatal("publisher shares writable build caches with analysis")
		}
	}
	if publishCommands != 1 {
		t.Fatal("workflow has no unique isolated publication step")
	}
}
