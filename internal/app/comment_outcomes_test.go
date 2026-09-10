package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/outcomes"
)

type outcomeReader struct {
	err error
}

func (reader outcomeReader) GetCommentOutcomes(_ context.Context, _, _ string, number int) (outcomes.Snapshot, error) {
	return outcomes.Snapshot{
		Number: number, Author: "owner", State: "MERGED", URL: "https://github.com/o/r/pull/7",
		Threads: []outcomes.Thread{{
			ID: "thread", Resolved: true,
			Comments: []outcomes.Comment{
				{ID: 1, Author: "reviewer", Body: "Repeated work.", Path: "src/file.ts", URL: "https://github.com/o/r/pull/7#discussion_r1"},
				{ID: 2, Author: "owner", Body: "Not a bug; this is cached."},
			},
		}},
	}, reader.err
}

type outcomeClassifier struct{}

func (outcomeClassifier) Assess(context.Context, outcomes.Input) (outcomes.Assessment, error) {
	return outcomes.Assessment{
		Feedback: "disagreed", Confidence: .95, Rationale: "Author challenges the finding.",
		Evidence: []outcomes.Evidence{{CommentID: 2, Quote: "Not a bug"}},
	}, nil
}

func TestTrackCommentOutcomesPersistsRoundTripAndFailureEvidence(t *testing.T) {
	root := t.TempDir()
	paths := OutcomePaths{JSON: filepath.Join(root, "ledger.json"), Markdown: filepath.Join(root, "stats.md")}
	options := outcomes.Options{Owner: "o", Repo: "r", Model: "model", MaxPulls: 10, MaxAssessments: 10}
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	run := func(reader outcomeReader) error {
		return trackCommentOutcomes(context.Background(), paths, []int{7, 7}, reader, outcomeClassifier{}, options, now)
	}
	if err := run(outcomeReader{}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(outcomeReader{}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("unchanged retry is not idempotent")
	}
	content, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"| resolved | 1 | 100.0% |", "| disagreed | 1 | 100.0% |",
		"Tracked comments: **1**", "Not a bug", "95% confidence", "Resolved does not mean fixed",
		"https://github.com/o/r/pull/7#discussion_r1", "(Written by Copilot)",
	} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("missing %q in %s", expected, content)
		}
	}
	if err := run(outcomeReader{err: errors.New("rate limited")}); err == nil {
		t.Fatal("refresh failure hidden")
	}
	var ledger outcomes.Ledger
	if err := decodeStrictJSONFile(paths.JSON, &ledger, maxOutcomeLedgerBytes); err != nil {
		t.Fatal(err)
	}
	if ledger.Statistics.Total != 1 || ledger.Statistics.Outcomes["unavailable"] != 1 ||
		ledger.Pulls[0].Comments[0].Assessment.Feedback != "disagreed" || ledger.Pulls[0].Error != "rate limited" {
		t.Fatalf("failed refresh lost evidence or counted stale result: %+v", ledger)
	}
}

func TestOutcomeDiscoveryUsesFullArchiveNotCappedIndex(t *testing.T) {
	root := t.TempDir()
	runPath := filepath.Join(root, "2026", "09", "10", "1-1", "run.json")
	if err := os.MkdirAll(filepath.Dir(runPath), 0o755); err != nil {
		t.Fatal(err)
	}
	record := PollRunRecord{Version: runRecordVersion}
	for number := 1; number <= maxPublishedReviewEntries+1; number++ {
		record.Reviews = append(record.Reviews, RunReviewRecord{Number: number, Published: true})
	}
	record.Reviews = append(record.Reviews, RunReviewRecord{Number: 1, Published: true}, RunReviewRecord{Number: 999})
	writeJSONFile(t, runPath, record)
	numbers, err := publishedPullNumbers(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(numbers) != maxPublishedReviewEntries+1 || numbers[0] != 1 || numbers[len(numbers)-1] != 201 {
		t.Fatalf("discovery lost older publications or duplicates: %v", numbers)
	}
}

func TestOutcomeLedgerRejectsInvalidInputWithoutOverwriting(t *testing.T) {
	for _, content := range []string{
		`{`,
		`{"version":99,"repository":"o/r"}`,
		`{"version":1,"repository":"wrong/repo"}`,
		`{"version":1,"repository":"o/r","unknown":true}`,
		`{"version":1,"repository":"o/r"} {}`,
	} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			paths := OutcomePaths{JSON: filepath.Join(root, "ledger.json"), Markdown: filepath.Join(root, "stats.md")}
			if err := os.WriteFile(paths.JSON, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := trackCommentOutcomes(context.Background(), paths, nil, outcomeReader{}, outcomeClassifier{},
				outcomes.Options{Owner: "o", Repo: "r", MaxPulls: 10, MaxAssessments: 10}, time.Now())
			if err == nil {
				t.Fatal("invalid ledger accepted")
			}
			saved, err := os.ReadFile(paths.JSON)
			if err != nil || string(saved) != content {
				t.Fatal("invalid ledger overwritten")
			}
		})
	}
}

func TestOutcomeLedgerRejectsSameOutputPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	err := trackCommentOutcomes(context.Background(), OutcomePaths{JSON: path, Markdown: path}, nil, nil, nil, outcomes.Options{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "different") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmptyOutcomeStatistics(t *testing.T) {
	text := formatCommentOutcomes(outcomes.Ledger{Version: outcomes.Version, Repository: "o/r"})
	if !strings.Contains(text, "Tracked comments: **0**") || !strings.Contains(text, "| ignored | 0 | 0.0% |") {
		t.Fatal(text)
	}
}
