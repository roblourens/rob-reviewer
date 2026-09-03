package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestWritePublishedReviewIndexIncludesOnlyPublishedComments(t *testing.T) {
	root := t.TempDir()
	runPath := filepath.Join(root, "run.json")
	indexPath := filepath.Join(root, "nested", "published-reviews.json")
	markdownPath := filepath.Join(root, "nested", "published-reviews.md")
	record := PollRunRecord{
		Version: runRecordVersion,
		Metadata: PollRunMetadata{
			RunID: "123", RunAttempt: 2,
			CompletedAt: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
		},
		Reviews: []RunReviewRecord{
			{
				Number: 7, Title: "Published", URL: "https://example.test/7",
				HeadSHA: "head-7", Published: true,
				PublishedComments: []RunCommentRecord{{
					Path: "src/file.ts", Line: 12, Side: review.SideRight,
					Body: "**Severity: medium**\n\nProblem.\n\n**Suggested fix:** Fix it.",
				}},
			},
			{Number: 8, Title: "Clean", Published: false},
		},
	}
	writeJSONFile(t, runPath, record)
	if err := WritePublishedReviewIndex(indexPath, markdownPath, runPath); err != nil {
		t.Fatal(err)
	}

	var index PublishedReviewIndex
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 1 || index.Entries[0].Number != 7 ||
		len(index.Entries[0].Comments) != 1 {
		t.Fatalf("index = %+v", index)
	}
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"#7 — Published",
		"Published comments: **1**",
		"runs/2026/09/03/123-2/run.md",
		"**Severity: medium**",
	} {
		if !strings.Contains(string(markdown), expected) {
			t.Fatalf("Markdown missing %q: %s", expected, markdown)
		}
	}
}

func TestWritePublishedReviewIndexIsIdempotentAndNewestFirst(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "published-reviews.json")
	markdownPath := filepath.Join(root, "published-reviews.md")
	runPath := filepath.Join(root, "run.json")
	first := PollRunRecord{
		Version: runRecordVersion,
		Metadata: PollRunMetadata{
			RunID: "100", RunAttempt: 1,
			CompletedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
		},
		Reviews: []RunReviewRecord{publishedReviewForTest(7, "old-head")},
	}
	writeJSONFile(t, runPath, first)
	if err := WritePublishedReviewIndex(indexPath, markdownPath, runPath); err != nil {
		t.Fatal(err)
	}
	if err := WritePublishedReviewIndex(indexPath, markdownPath, runPath); err != nil {
		t.Fatal(err)
	}

	second := PollRunRecord{
		Version: runRecordVersion,
		Metadata: PollRunMetadata{
			RunID: "101", RunAttempt: 1,
			CompletedAt: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
		},
		Reviews: []RunReviewRecord{publishedReviewForTest(8, "new-head")},
	}
	writeJSONFile(t, runPath, second)
	if err := WritePublishedReviewIndex(indexPath, markdownPath, runPath); err != nil {
		t.Fatal(err)
	}
	var index PublishedReviewIndex
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 2 || index.Entries[0].Number != 8 || index.Entries[1].Number != 7 {
		t.Fatalf("index = %+v", index)
	}
}

func TestWritePublishedReviewIndexFromArchiveRecoversEarlierRun(t *testing.T) {
	root := t.TempDir()
	archiveRoot := filepath.Join(root, "runs")
	indexPath := filepath.Join(root, "published-reviews.json")
	markdownPath := filepath.Join(root, "published-reviews.md")
	firstPath := filepath.Join(archiveRoot, "2026", "09", "02", "100-1", "run.json")
	secondPath := filepath.Join(archiveRoot, "2026", "09", "03", "101-1", "run.json")
	for _, item := range []struct {
		path   string
		record PollRunRecord
	}{
		{
			path: firstPath,
			record: PollRunRecord{
				Version: runRecordVersion,
				Metadata: PollRunMetadata{
					RunID: "100", RunAttempt: 1,
					CompletedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
				},
				Reviews: []RunReviewRecord{publishedReviewForTest(7, "first-head")},
			},
		},
		{
			path: secondPath,
			record: PollRunRecord{
				Version: runRecordVersion,
				Metadata: PollRunMetadata{
					RunID: "101", RunAttempt: 1,
					CompletedAt: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
				},
			},
		},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeJSONFile(t, item.path, item.record)
	}
	if err := WritePublishedReviewIndexFromArchive(indexPath, markdownPath, archiveRoot); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index PublishedReviewIndex
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 1 || index.Entries[0].Number != 7 {
		t.Fatalf("index = %+v", index)
	}
}

func TestWritePublishedCommentsFileIncludesCountInName(t *testing.T) {
	path, err := WritePublishedCommentsFile(t.TempDir(), PollRunRecord{
		Reviews: []RunReviewRecord{publishedReviewForTest(7, "head")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "published-comments-1.md" {
		t.Fatalf("path = %q", path)
	}
}

func TestWritePublishedReviewIndexBoundsHistory(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "nested", "published-reviews.json")
	markdownPath := filepath.Join(root, "nested", "published-reviews.md")
	runPath := filepath.Join(root, "run.json")
	index := PublishedReviewIndex{Version: publishedReviewIndexVersion}
	for number := 1; number <= maxPublishedReviewEntries; number++ {
		index.Entries = append(index.Entries, PublishedReviewEntry{
			RunID:       "old",
			RunAttempt:  1,
			CompletedAt: time.Date(2026, 9, 1, 0, 0, number, 0, time.UTC),
			Number:      number,
			HeadSHA:     "old-head",
		})
	}
	writeJSONFile(t, filepath.Join(root, "existing.json"), index)
	content, err := os.ReadFile(filepath.Join(root, "existing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	run := PollRunRecord{
		Version: runRecordVersion,
		Metadata: PollRunMetadata{
			RunID:       "new",
			RunAttempt:  1,
			CompletedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		},
		Reviews: []RunReviewRecord{publishedReviewForTest(999, "new-head")},
	}
	writeJSONFile(t, runPath, run)
	if err := WritePublishedReviewIndex(indexPath, markdownPath, runPath); err != nil {
		t.Fatal(err)
	}
	updatedContent, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var updated PublishedReviewIndex
	if err := json.Unmarshal(updatedContent, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Entries) != maxPublishedReviewEntries || updated.Entries[0].Number != 999 {
		t.Fatalf("bounded index has %d entries, first = %+v", len(updated.Entries), updated.Entries[0])
	}
}

func publishedReviewForTest(number int, head string) RunReviewRecord {
	return RunReviewRecord{
		Number: number, Title: "PR", URL: "https://example.test",
		HeadSHA: head, Published: true,
		PublishedComments: []RunCommentRecord{{
			Path: "src/file.ts", Line: 1, Side: review.SideRight, Body: "Comment",
		}},
	}
}

func writeJSONFile[T strictJSONTarget](t *testing.T, path string, value T) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
