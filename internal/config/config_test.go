package config

import (
	"strings"
	"testing"
)

func TestDecodeAppliesDefaults(t *testing.T) {
	cfg, err := Decode(strings.NewReader(`
version: 1
target:
  owner: microsoft
  repo: vscode
review:
  focuses:
    - performance-review
`))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Review.Model != DefaultModel {
		t.Fatalf("model = %q, want %q", cfg.Review.Model, DefaultModel)
	}
	if cfg.Poll.MaxPerRun != DefaultMaxPerRun {
		t.Fatalf("maxPerRun = %d, want %d", cfg.Poll.MaxPerRun, DefaultMaxPerRun)
	}
	if cfg.Review.MaxFindings != DefaultMaxFindings {
		t.Fatalf("maxFindings = %d, want %d", cfg.Review.MaxFindings, DefaultMaxFindings)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := Decode(strings.NewReader(`
version: 1
target:
  owner: microsoft
  repo: vscode
review:
  focuses:
    - performance-review
unknown: true
`))
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateRejectsDuplicateFocus(t *testing.T) {
	_, err := Decode(strings.NewReader(`
version: 1
target:
  owner: microsoft
  repo: vscode
review:
  focuses:
    - performance-review
    - performance-review
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate focus error, got %v", err)
	}
}
