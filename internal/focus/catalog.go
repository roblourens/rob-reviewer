package focus

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxSkillFileSize = 1 << 20

type Entry struct {
	Name      string
	Directory string
}

type Catalog struct {
	root    string
	entries map[string]Entry
}

type skillMetadata struct {
	Name string `yaml:"name"`
}

func Load(root string) (*Catalog, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve focus root: %w", err)
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve focus root symlinks: %w", err)
	}
	directories, err := os.ReadDir(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("read focus root %q: %w", absoluteRoot, err)
	}

	catalog := &Catalog{
		root:    absoluteRoot,
		entries: make(map[string]Entry),
	}
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		skillPath := filepath.Join(absoluteRoot, directory.Name(), "SKILL.md")
		metadata, err := readSkillMetadata(skillPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, exists := catalog.entries[metadata.Name]; exists {
			return nil, fmt.Errorf("duplicate skill name %q", metadata.Name)
		}
		catalog.entries[metadata.Name] = Entry{
			Name:      metadata.Name,
			Directory: filepath.Dir(skillPath),
		}
	}
	return catalog, nil
}

func (catalog *Catalog) Root() string {
	return catalog.root
}

func (catalog *Catalog) Resolve(names []string) ([]Entry, error) {
	result := make([]Entry, 0, len(names))
	for _, name := range names {
		entry, exists := catalog.entries[name]
		if !exists {
			return nil, fmt.Errorf("focus %q does not exist under %s", name, catalog.root)
		}
		result = append(result, entry)
	}
	return result, nil
}

func (catalog *Catalog) ReadDocument(focusName, relativePath string, maxBytes int) (string, error) {
	entry, exists := catalog.entries[focusName]
	if !exists {
		return "", fmt.Errorf("unknown focus %q", focusName)
	}
	if maxBytes < 1 {
		return "", errors.New("maxBytes must be positive")
	}
	cleanPath := filepath.Clean(relativePath)
	if filepath.IsAbs(cleanPath) || cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." {
		return "", fmt.Errorf("document path %q must stay inside focus %q", relativePath, focusName)
	}
	if !slices.Contains([]string{".md", ".txt"}, strings.ToLower(filepath.Ext(cleanPath))) {
		return "", fmt.Errorf("document %q must be Markdown or text", relativePath)
	}
	absolutePath, err := filepath.EvalSymlinks(filepath.Join(entry.Directory, cleanPath))
	if err != nil {
		return "", fmt.Errorf("resolve focus document: %w", err)
	}
	relative, err := filepath.Rel(entry.Directory, absolutePath)
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return "", fmt.Errorf("document path %q escapes focus %q", relativePath, focusName)
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return "", fmt.Errorf("open focus document: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes+1)))
	if err != nil {
		return "", fmt.Errorf("read focus document: %w", err)
	}
	if len(content) > maxBytes {
		return "", fmt.Errorf("focus document exceeds %d bytes", maxBytes)
	}
	return string(content), nil
}

func readSkillMetadata(path string) (skillMetadata, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return skillMetadata{}, err
	}
	if fileInfo.Size() > maxSkillFileSize {
		return skillMetadata{}, fmt.Errorf("skill file %q exceeds %d bytes", path, maxSkillFileSize)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return skillMetadata{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return skillMetadata{}, fmt.Errorf("skill %q is missing YAML frontmatter", path)
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return skillMetadata{}, fmt.Errorf("skill %q has unterminated YAML frontmatter", path)
	}
	var metadata skillMetadata
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &metadata); err != nil {
		return skillMetadata{}, fmt.Errorf("decode skill metadata %q: %w", path, err)
	}
	metadata.Name = strings.TrimSpace(metadata.Name)
	if metadata.Name == "" {
		return skillMetadata{}, fmt.Errorf("skill %q has no name", path)
	}
	return metadata, nil
}
