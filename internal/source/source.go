package source

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/diff"
	"github.com/roblourens/rob-reviewer/internal/focus"
	"github.com/roblourens/rob-reviewer/internal/review"
)

const (
	maxReadLines       = 400
	maxReadBytes       = 64 << 10
	maxReadableFile    = 10 << 20
	maxDiffBytes       = 128 << 10
	maxSearchPattern   = 200
	maxSearchResults   = 100
	maxSearchLineBytes = 4 << 10
	maxSearchStderr    = 64 << 10
	maxFocusBytes      = 64 << 10
)

type Source struct {
	root    string
	pull    review.PullRequest
	diff    *diff.Diff
	focuses *focus.Catalog
}

type PRContext struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	URL     string `json:"url"`
	BaseRef string `json:"baseRef"`
	BaseSHA string `json:"baseSha"`
	HeadRef string `json:"headRef"`
	HeadSHA string `json:"headSha"`
}

type FileContent struct {
	Path      string   `json:"path"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

type DiffContent struct {
	Path       string `json:"path"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"nextOffset"`
	Content    string `json:"content"`
	TotalBytes int    `json:"totalBytes"`
	Truncated  bool   `json:"truncated"`
}

type ChangedFilesPage struct {
	Files      []diff.ChangedFile `json:"files"`
	Total      int                `json:"total"`
	NextOffset int                `json:"nextOffset"`
}

type SearchMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

func New(root string, pull review.PullRequest, parsedDiff *diff.Diff, focuses *focus.Catalog) (*Source, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve source root symlinks: %w", err)
	}
	if parsedDiff == nil {
		return nil, errors.New("parsed diff is required")
	}
	if focuses == nil {
		return nil, errors.New("focus catalog is required")
	}
	return &Source{
		root:    absoluteRoot,
		pull:    pull,
		diff:    parsedDiff,
		focuses: focuses,
	}, nil
}

func (source *Source) PullRequestContext() PRContext {
	return PRContext{
		Number:  source.pull.Number,
		Title:   source.pull.Title,
		Body:    source.pull.Body,
		URL:     source.pull.URL,
		BaseRef: source.pull.BaseRef,
		BaseSHA: source.pull.BaseSHA,
		HeadRef: source.pull.HeadRef,
		HeadSHA: source.pull.HeadSHA,
	}
}

func (source *Source) Root() string {
	return source.root
}

func (source *Source) ChangedFiles(offset, limit int) (ChangedFilesPage, error) {
	if offset < 0 {
		return ChangedFilesPage{}, errors.New("offset cannot be negative")
	}
	if limit < 1 || limit > 200 {
		return ChangedFilesPage{}, errors.New("limit must be between 1 and 200")
	}
	files := source.diff.ChangedFiles()
	offset = min(offset, len(files))
	end := min(offset+limit, len(files))
	nextOffset := 0
	if end < len(files) {
		nextOffset = end
	}
	return ChangedFilesPage{
		Files:      files[offset:end],
		Total:      len(files),
		NextOffset: nextOffset,
	}, nil
}

func (source *Source) ReadDiff(path string, offset, maxBytes int) (DiffContent, error) {
	if maxBytes < 1 || maxBytes > maxDiffBytes {
		return DiffContent{}, fmt.Errorf("maxBytes must be between 1 and %d", maxDiffBytes)
	}
	result, err := source.diff.ReadFileRange(path, offset, maxBytes)
	if err != nil {
		return DiffContent{}, err
	}
	nextOffset := 0
	if result.Truncated {
		nextOffset = offset + len(result.Bytes)
	}
	return DiffContent{
		Path:       path,
		Offset:     offset,
		NextOffset: nextOffset,
		Content:    result.String(),
		TotalBytes: result.TotalBytes,
		Truncated:  result.Truncated,
	}, nil
}

func (source *Source) ReadFile(path string, startLine, endLine int) (FileContent, error) {
	if startLine < 1 {
		return FileContent{}, errors.New("startLine must be positive")
	}
	if endLine < startLine || endLine-startLine+1 > maxReadLines {
		return FileContent{}, fmt.Errorf("line range must contain 1-%d lines", maxReadLines)
	}
	absolutePath, err := source.resolvePath(path)
	if err != nil {
		return FileContent{}, err
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return FileContent{}, fmt.Errorf("open repository file %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return FileContent{}, fmt.Errorf("stat repository file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return FileContent{}, fmt.Errorf("repository path %q is not a regular file", path)
	}
	if info.Size() > maxReadableFile {
		return FileContent{}, fmt.Errorf("repository file %q exceeds %d bytes", path, maxReadableFile)
	}

	reader := bufio.NewReader(file)
	lines := make([]string, 0, endLine-startLine+1)
	lineNumber := 0
	returnedBytes := 0
	truncated := false
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNumber++
			if lineNumber >= startLine && lineNumber <= endLine {
				line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
				if strings.ContainsRune(line, '\x00') {
					return FileContent{}, fmt.Errorf("repository file %q appears to be binary", path)
				}
				if returnedBytes+len(line) > maxReadBytes {
					truncated = true
					break
				}
				returnedBytes += len(line)
				lines = append(lines, line)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return FileContent{}, fmt.Errorf("read repository file %q: %w", path, readErr)
			}
			break
		}
		if lineNumber >= endLine {
			break
		}
	}
	return FileContent{
		Path:      filepath.ToSlash(path),
		StartLine: startLine,
		EndLine:   startLine + len(lines) - 1,
		Lines:     lines,
		Truncated: truncated,
	}, nil
}

func (source *Source) Search(ctx context.Context, pattern, path string, literal bool, maxResults int) ([]SearchMatch, error) {
	if pattern == "" || len(pattern) > maxSearchPattern {
		return nil, fmt.Errorf("pattern must contain 1-%d characters", maxSearchPattern)
	}
	if !literal {
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid regular expression: %w", err)
		}
	}
	if maxResults < 1 || maxResults > maxSearchResults {
		return nil, fmt.Errorf("maxResults must be between 1 and %d", maxSearchResults)
	}
	searchRoot := source.root
	if strings.TrimSpace(path) != "" {
		resolved, err := source.resolvePath(path)
		if err != nil {
			return nil, err
		}
		searchRoot = resolved
	}

	args := []string{
		"--no-config",
		"--no-messages",
		"--with-filename",
		"--json",
		"--max-filesize", "2M",
		"--glob", "!.git/**",
	}
	if literal {
		args = append(args, "--fixed-strings")
	}
	args = append(args, "--", pattern, searchRoot)
	command := exec.CommandContext(ctx, "rg", args...)
	command.Dir = source.root
	command.Env = append(os.Environ(), "RIPGREP_CONFIG_PATH=")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open search output: %w", err)
	}
	stderr := newLimitedBuffer(maxSearchStderr)
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start repository search: %w", err)
	}
	matches, reachedLimit, decodeErr := source.decodeSearchJSON(json.NewDecoder(stdout), maxResults)
	if decodeErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, decodeErr
	}
	if reachedLimit {
		_ = command.Process.Kill()
		_ = command.Wait()
		return matches, nil
	}
	if err := command.Wait(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return []SearchMatch{}, nil
		}
		return nil, fmt.Errorf("search repository: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return matches, nil
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (buffer *limitedBuffer) Write(content []byte) (int, error) {
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		return 0, errors.New("search output exceeded limit")
	}
	if len(content) > remaining {
		written, _ := buffer.buffer.Write(content[:remaining])
		return written, errors.New("search output exceeded limit")
	}
	return buffer.buffer.Write(content)
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}

func (source *Source) ReadFocusDocument(focusName, path string) (string, error) {
	return source.focuses.ReadDocument(focusName, path, maxFocusBytes)
}

func (source *Source) ValidAnchor(path string, side review.Side, line int) bool {
	return source.diff.ValidateAnchor(diff.Anchor{Path: path, Side: side, Line: line}) == nil
}

func (source *Source) resolvePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(cleanPath) || cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q must stay inside the repository", path)
	}
	candidate := filepath.Join(source.root, cleanPath)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve repository path %q: %w", path, err)
	}
	relative, err := filepath.Rel(source.root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the repository", path)
	}
	return resolved, nil
}

type rgJSONEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func (source *Source) parseSearchJSON(content []byte, maxResults int) ([]SearchMatch, error) {
	matches, _, err := source.decodeSearchJSON(json.NewDecoder(bytes.NewReader(content)), maxResults)
	return matches, err
}

func (source *Source) decodeSearchJSON(decoder *json.Decoder, maxResults int) ([]SearchMatch, bool, error) {
	matches := make([]SearchMatch, 0, maxResults)
	for len(matches) < maxResults {
		var event rgJSONEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, false, fmt.Errorf("decode search output: %w", err)
		}
		if event.Type != "match" {
			continue
		}
		if event.Data.Path.Text == "" || len(event.Data.Submatches) == 0 {
			return nil, false, errors.New("search returned a non-text path or malformed match")
		}
		relative, err := filepath.Rel(source.root, event.Data.Path.Text)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, false, fmt.Errorf("search result path %q escapes repository", event.Data.Path.Text)
		}
		text := strings.TrimSuffix(strings.TrimSuffix(event.Data.Lines.Text, "\n"), "\r")
		if len(text) > maxSearchLineBytes {
			text = text[:maxSearchLineBytes]
		}
		matches = append(matches, SearchMatch{
			Path:   filepath.ToSlash(relative),
			Line:   event.Data.LineNumber,
			Column: event.Data.Submatches[0].Start + 1,
			Text:   text,
		})
	}
	return matches, len(matches) == maxResults, nil
}
