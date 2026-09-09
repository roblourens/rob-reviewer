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
	if cfg.Poll.MaxPerDay != DefaultMaxPerDay ||
		cfg.Poll.ScanWindowHours != DefaultScanHours ||
		cfg.Poll.MaxPendingHours != DefaultMaxPendingHours {
		t.Fatalf("poll defaults = %+v", cfg.Poll)
	}
	if cfg.Automation.Enabled || cfg.Publication.Mode != "approval" {
		t.Fatalf("safe automation defaults = %+v / %+v", cfg.Automation, cfg.Publication)
	}
	if !cfg.Learning.Enabled || cfg.Learning.MaxCasesPerRun != 1 {
		t.Fatalf("learning defaults = %+v", cfg.Learning)
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

func TestValidateRejectsUnsafeAutomationLimitsAndPublicationMode(t *testing.T) {
	_, err := Decode(strings.NewReader(`
version: 1
target:
  owner: microsoft
  repo: vscode
poll:
  maxPerRun: 5
  maxPerDay: 4
  scanWindowHours: 0
  maxPendingAgeHours: 0
automation:
  skipLabels:
    - skip
    - skip
publication:
  mode: publish-everything
review:
  focuses:
    - performance-review
`))
	if err == nil {
		t.Fatal("expected automation validation errors")
	}
	for _, expected := range []string{
		"maxPerDay",
		"scanWindowHours",
		"maxPendingAgeHours",
		"duplicate",
		"publication.mode",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q does not contain %q", err, expected)
		}
	}
}
