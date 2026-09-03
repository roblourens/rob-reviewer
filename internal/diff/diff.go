package diff

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type LineKind string

const (
	LineContext  LineKind = "context"
	LineAddition LineKind = "addition"
	LineDeletion LineKind = "deletion"
)

type Status string

const (
	StatusModified Status = "modified"
	StatusAdded    Status = "added"
	StatusDeleted  Status = "deleted"
	StatusRenamed  Status = "renamed"
)

type Line struct {
	Kind    LineKind
	Content string
	OldLine int
	NewLine int
}

type Hunk struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []Line

	raw []byte
}

type File struct {
	Path     string
	OldPath  string
	NewPath  string
	Status   Status
	Binary   bool
	HasPatch bool
	Hunks    []Hunk

	raw []byte
}

type ChangedFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath"`
	NewPath   string `json:"newPath"`
	Status    Status `json:"status"`
	Binary    bool   `json:"binary"`
	HasPatch  bool   `json:"hasPatch"`
	HunkCount int    `json:"hunkCount"`
}

type Anchor struct {
	Path string
	Side review.Side
	Line int
}

type ReadResult struct {
	Bytes      []byte
	TotalBytes int
	Truncated  bool
}

func (r ReadResult) String() string {
	return string(r.Bytes)
}

type Diff struct {
	Files []File
}

const (
	maxParsedDiffBytes = 32 << 20
	maxParsedDiffLines = 200_000
	maxParsedFiles     = 10_000
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?: .*)?$`)

func Parse(data []byte) (*Diff, error) {
	if len(data) > maxParsedDiffBytes {
		return nil, fmt.Errorf("diff exceeds %d-byte parse limit", maxParsedDiffBytes)
	}
	lineCount := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lineCount++
	}
	if lineCount > maxParsedDiffLines {
		return nil, fmt.Errorf("diff exceeds %d-line parse limit", maxParsedDiffLines)
	}
	lines := splitLines(data)
	result := &Diff{}
	for index := 0; index < len(lines); {
		if !strings.HasPrefix(lineText(lines[index]), "diff --git ") {
			if strings.TrimSpace(lineText(lines[index])) != "" {
				return nil, fmt.Errorf("line %d: expected diff --git header", index+1)
			}
			index++
			continue
		}

		end := index + 1
		for end < len(lines) && !strings.HasPrefix(lineText(lines[end]), "diff --git ") {
			end++
		}
		file, err := parseFile(lines[index:end], index+1)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, file)
		if len(result.Files) > maxParsedFiles {
			return nil, fmt.Errorf("diff exceeds %d-file parse limit", maxParsedFiles)
		}
		index = end
	}
	return result, nil
}

func ParseString(text string) (*Diff, error) {
	return Parse([]byte(text))
}

func (d *Diff) ChangedFiles() []ChangedFile {
	files := make([]ChangedFile, 0, len(d.Files))
	for _, file := range d.Files {
		files = append(files, ChangedFile{
			Path:      file.Path,
			OldPath:   file.OldPath,
			NewPath:   file.NewPath,
			Status:    file.Status,
			Binary:    file.Binary,
			HasPatch:  file.HasPatch,
			HunkCount: len(file.Hunks),
		})
	}
	return files
}

func (d *Diff) ValidateAnchor(anchor Anchor) error {
	if anchor.Line < 1 {
		return errors.New("anchor line must be positive")
	}

	file := d.findFile(anchor.Path)
	if file == nil {
		return fmt.Errorf("path %q is not in the diff", anchor.Path)
	}
	if file.Binary || !file.HasPatch {
		return fmt.Errorf("path %q has no reviewable patch", anchor.Path)
	}
	if anchor.Side != review.SideLeft && anchor.Side != review.SideRight {
		return fmt.Errorf("invalid anchor side %q", anchor.Side)
	}

	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			switch anchor.Side {
			case review.SideLeft:
				if line.OldLine == anchor.Line && line.Kind == LineDeletion {
					return nil
				}
			case review.SideRight:
				if line.NewLine == anchor.Line && line.Kind == LineAddition {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("%s line %d is not present in the patch for %q", anchor.Side, anchor.Line, anchor.Path)
}

func (d *Diff) ContainsPath(path string) bool {
	for _, file := range d.Files {
		if path == file.Path || path == file.OldPath || path == file.NewPath {
			return true
		}
	}
	return false
}

func (d *Diff) ReadFile(path string, maxBytes int) (ReadResult, error) {
	return d.ReadFileRange(path, 0, maxBytes)
}

func (d *Diff) ReadFileRange(path string, offset, maxBytes int) (ReadResult, error) {
	if offset < 0 {
		return ReadResult{}, errors.New("offset cannot be negative")
	}
	if maxBytes < 0 {
		return ReadResult{}, errors.New("maxBytes cannot be negative")
	}
	file := d.findFile(path)
	if file == nil {
		return ReadResult{}, fmt.Errorf("path %q is not in the diff", path)
	}
	return boundedReadRange(file.raw, offset, maxBytes), nil
}

func (d *Diff) ReadHunk(path string, hunkIndex, maxBytes int) (ReadResult, error) {
	if maxBytes < 0 {
		return ReadResult{}, errors.New("maxBytes cannot be negative")
	}
	file := d.findFile(path)
	if file == nil {
		return ReadResult{}, fmt.Errorf("path %q is not in the diff", path)
	}
	if hunkIndex < 0 || hunkIndex >= len(file.Hunks) {
		return ReadResult{}, fmt.Errorf("hunk index %d out of range for %q", hunkIndex, path)
	}
	return boundedRead(file.Hunks[hunkIndex].raw, maxBytes), nil
}

func (d *Diff) findFile(path string) *File {
	for index := range d.Files {
		if d.Files[index].Path == path {
			return &d.Files[index]
		}
	}
	return nil
}

func boundedRead(data []byte, maxBytes int) ReadResult {
	return boundedReadRange(data, 0, maxBytes)
}

func boundedReadRange(data []byte, offset, maxBytes int) ReadResult {
	offset = min(offset, len(data))
	size := min(len(data)-offset, maxBytes)
	content := make([]byte, size)
	copy(content, data[offset:offset+size])
	return ReadResult{
		Bytes:      content,
		TotalBytes: len(data),
		Truncated:  offset+size < len(data),
	}
}

func parseFile(lines [][]byte, firstLine int) (File, error) {
	oldPath, newPath, err := parseDiffHeader(lineText(lines[0]))
	if err != nil {
		return File{}, fmt.Errorf("line %d: %w", firstLine, err)
	}
	file := File{OldPath: oldPath, NewPath: newPath, raw: bytes.Join(lines, nil)}

	for index := 1; index < len(lines); {
		text := lineText(lines[index])
		switch {
		case strings.HasPrefix(text, "new file mode "):
			file.OldPath = ""
			index++
		case strings.HasPrefix(text, "deleted file mode "):
			file.NewPath = ""
			index++
		case strings.HasPrefix(text, "rename from "):
			file.OldPath, err = parsePath(strings.TrimPrefix(text, "rename from "), false)
			if err != nil {
				return File{}, fmt.Errorf("line %d: parse rename source: %w", firstLine+index, err)
			}
			index++
		case strings.HasPrefix(text, "rename to "):
			file.NewPath, err = parsePath(strings.TrimPrefix(text, "rename to "), false)
			if err != nil {
				return File{}, fmt.Errorf("line %d: parse rename destination: %w", firstLine+index, err)
			}
			index++
		case strings.HasPrefix(text, "--- "):
			file.OldPath, err = parsePathHeader(strings.TrimPrefix(text, "--- "), "a/")
			if err != nil {
				return File{}, fmt.Errorf("line %d: parse old path: %w", firstLine+index, err)
			}
			index++
		case strings.HasPrefix(text, "+++ "):
			file.NewPath, err = parsePathHeader(strings.TrimPrefix(text, "+++ "), "b/")
			if err != nil {
				return File{}, fmt.Errorf("line %d: parse new path: %w", firstLine+index, err)
			}
			index++
		case strings.HasPrefix(text, "@@ "):
			hunk, consumed, parseErr := parseHunk(lines[index:], firstLine+index)
			if parseErr != nil {
				return File{}, parseErr
			}
			file.Hunks = append(file.Hunks, hunk)
			file.HasPatch = true
			index += consumed
		case text == "GIT binary patch" || strings.HasPrefix(text, "Binary files "):
			file.Binary = true
			index++
		default:
			index++
		}
	}

	switch {
	case file.NewPath == "":
		file.Path = file.OldPath
		file.Status = StatusDeleted
	case file.OldPath == "":
		file.Path = file.NewPath
		file.Status = StatusAdded
	case file.OldPath != file.NewPath:
		file.Path = file.NewPath
		file.Status = StatusRenamed
	default:
		file.Path = file.NewPath
		file.Status = StatusModified
	}
	if file.Path == "" {
		return File{}, fmt.Errorf("line %d: diff has no usable path", firstLine)
	}
	return file, nil
}

func parseHunk(lines [][]byte, firstLine int) (Hunk, int, error) {
	header := lineText(lines[0])
	match := hunkHeaderPattern.FindStringSubmatch(header)
	if match == nil {
		return Hunk{}, 0, fmt.Errorf("line %d: malformed hunk header %q", firstLine, header)
	}
	oldStart, oldCount, err := parseRange(match[1], match[2])
	if err != nil {
		return Hunk{}, 0, fmt.Errorf("line %d: parse old range: %w", firstLine, err)
	}
	newStart, newCount, err := parseRange(match[3], match[4])
	if err != nil {
		return Hunk{}, 0, fmt.Errorf("line %d: parse new range: %w", firstLine, err)
	}

	hunk := Hunk{
		Header:   header,
		OldStart: oldStart,
		OldLines: oldCount,
		NewStart: newStart,
		NewLines: newCount,
	}
	oldLine, newLine := oldStart, newStart
	oldSeen, newSeen := 0, 0
	index := 1
	for index < len(lines) && (oldSeen < oldCount || newSeen < newCount) {
		text := lineText(lines[index])
		if text == `\ No newline at end of file` {
			index++
			continue
		}
		if text == "" {
			return Hunk{}, 0, fmt.Errorf("line %d: empty line in hunk must have a prefix", firstLine+index)
		}
		line := Line{Content: text[1:]}
		switch text[0] {
		case ' ':
			line.Kind = LineContext
			line.OldLine = oldLine
			line.NewLine = newLine
			oldLine++
			newLine++
			oldSeen++
			newSeen++
		case '-':
			line.Kind = LineDeletion
			line.OldLine = oldLine
			oldLine++
			oldSeen++
		case '+':
			line.Kind = LineAddition
			line.NewLine = newLine
			newLine++
			newSeen++
		default:
			return Hunk{}, 0, fmt.Errorf("line %d: invalid hunk line prefix %q", firstLine+index, text[0])
		}
		if oldSeen > oldCount || newSeen > newCount {
			return Hunk{}, 0, fmt.Errorf("line %d: hunk contains more lines than its header declares", firstLine+index)
		}
		hunk.Lines = append(hunk.Lines, line)
		index++
	}
	if oldSeen != oldCount || newSeen != newCount {
		return Hunk{}, 0, fmt.Errorf(
			"line %d: hunk line counts are old=%d/%d new=%d/%d",
			firstLine, oldSeen, oldCount, newSeen, newCount,
		)
	}
	if index < len(lines) && lineText(lines[index]) == `\ No newline at end of file` {
		index++
	}
	hunk.raw = bytes.Join(lines[:index], nil)
	return hunk, index, nil
}

func parseRange(startText, countText string) (int, int, error) {
	start, err := strconv.Atoi(startText)
	if err != nil {
		return 0, 0, err
	}
	count := 1
	if countText != "" {
		count, err = strconv.Atoi(countText)
		if err != nil {
			return 0, 0, err
		}
	}
	return start, count, nil
}

func parseDiffHeader(header string) (string, string, error) {
	fields, err := splitGitFields(strings.TrimPrefix(header, "diff --git "))
	if err != nil {
		return "", "", fmt.Errorf("parse diff header: %w", err)
	}
	if len(fields) != 2 {
		return "", "", fmt.Errorf("malformed diff header %q", header)
	}
	oldPath, err := parsePath(fields[0], true)
	if err != nil {
		return "", "", err
	}
	newPath, err := parsePath(fields[1], true)
	if err != nil {
		return "", "", err
	}
	return strings.TrimPrefix(oldPath, "a/"), strings.TrimPrefix(newPath, "b/"), nil
}

func parsePathHeader(value, prefix string) (string, error) {
	if value == "/dev/null" {
		return "", nil
	}
	path, err := parsePath(value, true)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(path, prefix), nil
}

func parsePath(value string, singleField bool) (string, error) {
	value = strings.TrimSpace(value)
	if singleField && !strings.HasPrefix(value, `"`) {
		if tab := strings.IndexByte(value, '\t'); tab >= 0 {
			value = value[:tab]
		}
	}
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid quoted path %q: %w", value, err)
		}
		return unquoted, nil
	}
	if value == "" {
		return "", errors.New("empty path")
	}
	return value, nil
}

func splitGitFields(value string) ([]string, error) {
	var fields []string
	for index := 0; index < len(value); {
		for index < len(value) && value[index] == ' ' {
			index++
		}
		if index == len(value) {
			break
		}
		start := index
		if value[index] != '"' {
			for index < len(value) && value[index] != ' ' {
				index++
			}
		} else {
			index++
			escaped := false
			for index < len(value) {
				switch {
				case escaped:
					escaped = false
				case value[index] == '\\':
					escaped = true
				case value[index] == '"':
					index++
					goto fieldComplete
				}
				index++
			}
			return nil, errors.New("unterminated quoted path")
		}
	fieldComplete:
		fields = append(fields, value[start:index])
	}
	return fields, nil
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(data, []byte{'\n'})
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineText(line []byte) string {
	return strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
}
