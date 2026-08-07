package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/diff"
	"github.com/roblourens/rob-reviewer/internal/focus"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestReadFileAndAnchorContainment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.ts"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	focusRoot := filepath.Join(root, "focuses")
	if err := os.MkdirAll(filepath.Join(focusRoot, "performance"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(focusRoot, "performance", "SKILL.md"), []byte("---\nname: performance-review\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := focus.Load(focusRoot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.ParseString("diff --git a/file.ts b/file.ts\n--- a/file.ts\n+++ b/file.ts\n@@ -1 +1 @@\n-old\n+new\n")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(root, review.PullRequest{}, parsed, catalog)
	if err != nil {
		t.Fatal(err)
	}

	content, err := source.ReadFile("file.ts", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(content.Lines, ",") != "two,three" {
		t.Fatalf("lines = %v", content.Lines)
	}
	if !source.ValidAnchor("file.ts", review.SideRight, 1) {
		t.Fatal("expected changed right-side anchor")
	}
	if source.ValidAnchor("file.ts", review.SideLeft, 2) {
		t.Fatal("unexpected unchanged anchor")
	}
	if _, err := source.ReadFile("../outside", 1, 1); err == nil {
		t.Fatal("expected path containment error")
	}
}

func TestReadFileCanReachPastFirst64KiB(t *testing.T) {
	root := t.TempDir()
	var content strings.Builder
	for line := 1; line <= 8_000; line++ {
		fmt.Fprintf(&content, "line-%05d\n", line)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	source := testSource(t, root)
	result, err := source.ReadFile("large.txt", 7_500, 7_501)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Lines, ",") != "line-07500,line-07501" {
		t.Fatalf("lines = %v", result.Lines)
	}
}

func TestParseSearchJSONSupportsColonInPath(t *testing.T) {
	root := t.TempDir()
	source := testSource(t, root)
	event := rgJSONEvent{Type: "match"}
	event.Data.Path.Text = filepath.Join(source.root, "name:with-colon.ts")
	event.Data.Lines.Text = strings.Repeat("x", maxSearchLineBytes+10)
	event.Data.LineNumber = 7
	event.Data.Submatches = append(event.Data.Submatches, struct {
		Start int `json:"start"`
	}{Start: 3})
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := source.parseSearchJSON(append(encoded, '\n'), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Path != "name:with-colon.ts" || len(matches[0].Text) != maxSearchLineBytes {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestSearchStopsAtGlobalResultLimit(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 100; index++ {
		path := filepath.Join(root, fmt.Sprintf("file-%03d.txt", index))
		if err := os.WriteFile(path, []byte("needle\nneedle\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := testSource(t, root)
	matches, err := source.Search(context.Background(), "needle", "", true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
}

func testSource(t *testing.T, root string) *Source {
	t.Helper()
	focusRoot := filepath.Join(root, "focuses")
	if err := os.MkdirAll(filepath.Join(focusRoot, "performance"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(focusRoot, "performance", "SKILL.md"), []byte("---\nname: performance-review\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := focus.Load(focusRoot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.ParseString("")
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(root, review.PullRequest{}, parsed, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
