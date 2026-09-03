package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/poller"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestWritePollRunFilesRecordsPublishedComments(t *testing.T) {
	result := savedResult()
	directory := t.TempDir()
	metadata := PollRunMetadata{
		RunID:       "123",
		RunAttempt:  2,
		EventName:   "schedule",
		Workflow:    "Review VS Code pull requests",
		Repository:  "roblourens/rob-reviewer",
		CommitSHA:   "reviewer-sha",
		StartedAt:   time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 2, 12, 2, 0, 0, time.UTC),
	}
	paths, err := WritePollRunFiles(directory, PollResult{
		Poll: poller.Result{
			Reviewed:  []int{7},
			Published: []int{7},
			Deferred:  []int{8},
			Skipped:   []int{9},
		},
		Reviews: []review.Result{result},
	}, nil, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	content, err := os.ReadFile(filepath.Join(directory, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record PollRunRecord
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatal(err)
	}
	if !record.Succeeded || len(record.Reviews) != 1 || !record.Reviews[0].Published ||
		len(record.Reviews[0].PublishedComments) != 1 {
		t.Fatalf("record = %+v", record)
	}
	comment := record.Reviews[0].PublishedComments[0]
	for _, expected := range []string{"**Severity: high**", "Repeated work", "(Written by Copilot)"} {
		if !strings.Contains(comment.Body, expected) {
			t.Fatalf("comment %q is missing %q", comment.Body, expected)
		}
	}
	markdown, err := os.ReadFile(filepath.Join(directory, "run.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Reviewer run 123", "Published: 1", "Published comment", "**Severity: high**"} {
		if !strings.Contains(string(markdown), expected) {
			t.Fatalf("Markdown is missing %q: %s", expected, markdown)
		}
	}
}

func TestWritePollRunFilesRecordsPartialFailure(t *testing.T) {
	directory := t.TempDir()
	expected := errors.New("review PR 7: model failed")
	_, err := WritePollRunFiles(directory, PollResult{
		Poll: poller.Result{Reviewed: []int{6}},
	}, expected, PollRunMetadata{
		RunID:       "failed-run",
		StartedAt:   time.Now().Add(-time.Minute),
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record PollRunRecord
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatal(err)
	}
	if record.Succeeded || record.Error != expected.Error() || len(record.Reviewed) != 1 {
		t.Fatalf("record = %+v", record)
	}
}
