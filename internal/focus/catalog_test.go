package focus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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

func TestMarketplaceInstallsTheHostedSkillAndItsReferences(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	content, err := os.ReadFile(filepath.Join(root, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	var marketplace struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Source  string `json:"source"`
		} `json:"plugins"`
	}
	content, err = os.ReadFile(filepath.Join(root, ".github", "plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &marketplace); err != nil {
		t.Fatal(err)
	}
	if len(marketplace.Plugins) != 1 {
		t.Fatalf("expected one installable plugin, got %d", len(marketplace.Plugins))
	}
	plugin := marketplace.Plugins[0]
	if plugin.Name != manifest.Name || plugin.Version != manifest.Version || manifest.Version == "" {
		t.Fatal("marketplace and plugin identity/version differ")
	}
	installedRoot := filepath.Join(root, plugin.Source)
	if installedRoot != root {
		t.Fatal("marketplace does not install the shared plugin root")
	}
	catalog, err := Load(filepath.Join(installedRoot, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.Resolve([]string{"performance-review"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("plugin skill resolution: entries=%v err=%v", entries, err)
	}
	if filepath.Base(entries[0].Directory) != entries[0].Name {
		t.Fatal("portable skill name does not match its directory")
	}
	skill, err := catalog.ReadDocument(entries[0].Name, "SKILL.md", maxSkillFileSize)
	if err != nil {
		t.Fatal(err)
	}
	for _, hostTool := range []string{"report_regression_fix", "blame_base_line", "report_finding", "complete_review"} {
		if strings.Contains(skill, hostTool) {
			t.Fatalf("standalone skill requires hosted tool %s", hostTool)
		}
	}
	links := regexp.MustCompile(`\]\((references/[^)]+)\)`).FindAllStringSubmatch(skill, -1)
	if len(links) < 4 {
		t.Fatal("standalone skill is missing the review guide, learnings, research, or case references")
	}
	for _, link := range links {
		if _, err := catalog.ReadDocument(entries[0].Name, link[1], maxSkillFileSize); err != nil {
			t.Fatalf("installed reference %s: %v", link[1], err)
		}
	}
}
