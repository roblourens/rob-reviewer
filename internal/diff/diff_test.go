package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

const syntheticDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -10,4 +10,5 @@ func main() {
 context one
-deleted
+added one
+added two
 context two
 context three
diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
--- a/old.txt
+++ b/new.txt
@@ -1,2 +1,2 @@
 same
-old
+new
diff --git a/image.png b/image.png
index 1111111..2222222 100644
Binary files a/image.png and b/image.png differ
diff --git a/mode.sh b/mode.sh
old mode 100644
new mode 100755
diff --git a/created.txt b/created.txt
new file mode 100644
index 0000000..1111111
--- /dev/null
+++ b/created.txt
@@ -0,0 +1 @@
+created
diff --git a/removed.txt b/removed.txt
deleted file mode 100644
index 1111111..0000000
--- a/removed.txt
+++ /dev/null
@@ -1 +0,0 @@
-removed
`

func TestParseLineMappings(t *testing.T) {
	parsed, err := ParseString(syntheticDiff)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Files) != 6 {
		t.Fatalf("files = %d, want 6", len(parsed.Files))
	}

	file := parsed.Files[0]
	if file.Path != "main.go" || file.Status != StatusModified || len(file.Hunks) != 1 {
		t.Fatalf("unexpected file: %+v", file)
	}
	lines := file.Hunks[0].Lines
	assertLine(t, lines[0], LineContext, 10, 10)
	assertLine(t, lines[1], LineDeletion, 11, 0)
	assertLine(t, lines[2], LineAddition, 0, 11)
	assertLine(t, lines[3], LineAddition, 0, 12)
	assertLine(t, lines[4], LineContext, 12, 13)
	assertLine(t, lines[5], LineContext, 13, 14)
}

func TestParseRenameBinaryAndNoPatch(t *testing.T) {
	parsed, err := ParseString(syntheticDiff)
	if err != nil {
		t.Fatal(err)
	}
	inventory := parsed.ChangedFiles()

	rename := inventory[1]
	if rename.Path != "new.txt" || rename.OldPath != "old.txt" || rename.Status != StatusRenamed || !rename.HasPatch {
		t.Fatalf("unexpected rename: %+v", rename)
	}
	binary := inventory[2]
	if !binary.Binary || binary.HasPatch {
		t.Fatalf("unexpected binary: %+v", binary)
	}
	noPatch := inventory[3]
	if noPatch.Binary || noPatch.HasPatch || noPatch.Status != StatusModified {
		t.Fatalf("unexpected no-patch file: %+v", noPatch)
	}
	added := inventory[4]
	if added.Path != "created.txt" || added.Status != StatusAdded {
		t.Fatalf("unexpected added file: %+v", added)
	}
	deleted := inventory[5]
	if deleted.Path != "removed.txt" || deleted.Status != StatusDeleted {
		t.Fatalf("unexpected deleted file: %+v", deleted)
	}
}

func TestValidateAnchor(t *testing.T) {
	parsed, err := ParseString(syntheticDiff)
	if err != nil {
		t.Fatal(err)
	}

	valid := []Anchor{
		{Path: "main.go", Side: review.SideLeft, Line: 11},
		{Path: "main.go", Side: review.SideRight, Line: 11},
		{Path: "new.txt", Side: review.SideRight, Line: 2},
		{Path: "created.txt", Side: review.SideRight, Line: 1},
		{Path: "removed.txt", Side: review.SideLeft, Line: 1},
	}
	for _, anchor := range valid {
		if err := parsed.ValidateAnchor(anchor); err != nil {
			t.Errorf("ValidateAnchor(%+v): %v", anchor, err)
		}
	}

	invalid := []Anchor{
		{Path: "missing.go", Side: review.SideRight, Line: 1},
		{Path: "main.go", Side: review.SideLeft, Line: 0},
		{Path: "main.go", Side: review.SideLeft, Line: 14},
		{Path: "main.go", Side: review.SideRight, Line: 15},
		{Path: "main.go", Side: review.SideLeft, Line: 12},
		{Path: "main.go", Side: review.SideRight, Line: 13},
		{Path: "image.png", Side: review.SideRight, Line: 1},
		{Path: "mode.sh", Side: review.SideLeft, Line: 1},
		{Path: "main.go", Side: review.Side("MIDDLE"), Line: 10},
	}
	for _, anchor := range invalid {
		if err := parsed.ValidateAnchor(anchor); err == nil {
			t.Errorf("ValidateAnchor(%+v) unexpectedly succeeded", anchor)
		}
	}
}

func TestParseRejectsExcessiveLineCount(t *testing.T) {
	data := bytes.Repeat([]byte("\n"), maxParsedDiffLines+1)
	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "line parse limit") {
		t.Fatalf("expected line limit error, got %v", err)
	}
}

func TestBoundedReads(t *testing.T) {
	parsed, err := ParseString(syntheticDiff)
	if err != nil {
		t.Fatal(err)
	}
	fileRead, err := parsed.ReadFile("main.go", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(fileRead.Bytes) != 20 || !fileRead.Truncated || fileRead.TotalBytes <= len(fileRead.Bytes) {
		t.Fatalf("unexpected bounded file read: %+v", fileRead)
	}
	hunkRead, err := parsed.ReadHunk("main.go", 0, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if hunkRead.Truncated || !strings.HasPrefix(hunkRead.String(), "@@ -10,4 +10,5 @@") {
		t.Fatalf("unexpected hunk read: %+v", hunkRead)
	}
}

func TestParseRejectsIncorrectHunkCounts(t *testing.T) {
	_, err := ParseString(`diff --git a/a b/a
--- a/a
+++ b/a
@@ -1,2 +1,1 @@
-old
+new
`)
	if err == nil || !strings.Contains(err.Error(), "hunk line counts") {
		t.Fatalf("expected hunk count error, got %v", err)
	}
}

func assertLine(t *testing.T, line Line, kind LineKind, oldLine, newLine int) {
	t.Helper()
	if line.Kind != kind || line.OldLine != oldLine || line.NewLine != newLine {
		t.Fatalf("line = %+v, want kind=%s old=%d new=%d", line, kind, oldLine, newLine)
	}
}
