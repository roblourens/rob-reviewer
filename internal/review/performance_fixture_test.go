package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/diff"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestPerformanceFixturesExposeOnlyChangedAnchors(t *testing.T) {
	for _, name := range []string{"per-message-write.diff", "batched-write.diff"} {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", "performance", name))
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := diff.Parse(content)
			if err != nil {
				t.Fatal(err)
			}
			files := parsed.ChangedFiles()
			if len(files) != 1 || files[0].Path != "src/logger.ts" {
				t.Fatalf("changed files = %+v", files)
			}
			if err := parsed.ValidateAnchor(diff.Anchor{Path: "src/logger.ts", Side: review.SideRight, Line: 12}); err != nil {
				t.Fatalf("expected changed-line anchor: %v", err)
			}
			if err := parsed.ValidateAnchor(diff.Anchor{Path: "src/logger.ts", Side: review.SideRight, Line: 10}); err == nil {
				t.Fatal("context line unexpectedly accepted as a finding anchor")
			}
		})
	}
}
