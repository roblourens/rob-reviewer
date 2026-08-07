package focus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogLoadsAndReadsDocument(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "performance")
	if err := os.MkdirAll(filepath.Join(directory, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: performance-review\n---\n# Skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "references", "evidence.md"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.Resolve([]string{"performance-review"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
	content, err := catalog.ReadDocument("performance-review", "references/evidence.md", 100)
	if err != nil || content != "evidence" {
		t.Fatalf("content = %q, err = %v", content, err)
	}
}

func TestCatalogRejectsEscapingDocument(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "performance")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: performance-review\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.ReadDocument("performance-review", "../outside.md", 100)
	if err == nil || !strings.Contains(err.Error(), "stay inside") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestCatalogRejectsSymlinkEscapingDocument(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "performance")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: performance-review\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "linked.md")); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ReadDocument("performance-review", "linked.md", 100); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected symlink containment error, got %v", err)
	}
}
