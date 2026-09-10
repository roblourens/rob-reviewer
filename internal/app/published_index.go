package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/outcomes"
	"github.com/roblourens/rob-reviewer/internal/review"
)

const (
	publishedReviewIndexVersion  = 1
	maxPublishedReviewEntries    = 200
	maxPublishedReviewIndexBytes = 64 << 20
)

type PublishedReviewIndex struct {
	Version int                    `json:"version"`
	Entries []PublishedReviewEntry `json:"entries"`
}

type PublishedReviewEntry struct {
	RunID          string             `json:"runId"`
	RunAttempt     int                `json:"runAttempt"`
	CompletedAt    time.Time          `json:"completedAt"`
	RunArchivePath string             `json:"runArchivePath"`
	Number         int                `json:"number"`
	Title          string             `json:"title"`
	URL            string             `json:"url"`
	HeadSHA        string             `json:"headSha"`
	Comments       []RunCommentRecord `json:"comments"`
}

func WritePublishedReviewIndex(jsonPath, markdownPath, runPath string) error {
	return writePublishedReviewIndex(jsonPath, markdownPath, []string{runPath})
}

func WritePublishedReviewIndexFromArchive(jsonPath, markdownPath, archiveRoot string) error {
	runPaths, err := archivedRunPaths(archiveRoot)
	if err != nil {
		return err
	}
	return writePublishedReviewIndex(jsonPath, markdownPath, runPaths)
}

func archivedRunPaths(archiveRoot string) ([]string, error) {
	var runPaths []string
	err := filepath.WalkDir(archiveRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "run.json" {
			runPaths = append(runPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find archived poll run records: %w", err)
	}
	slices.Sort(runPaths)
	return runPaths, nil
}

func writePublishedReviewIndex(jsonPath, markdownPath string, runPaths []string) error {
	index, err := loadPublishedReviewIndex(jsonPath)
	if err != nil {
		return err
	}
	for _, runPath := range runPaths {
		run, err := loadPollRunRecord(runPath)
		if err != nil {
			return err
		}
		for _, reviewed := range run.Reviews {
			if !reviewed.Published || len(reviewed.PublishedComments) == 0 {
				continue
			}
			entry := PublishedReviewEntry{
				RunID:          run.Metadata.RunID,
				RunAttempt:     run.Metadata.RunAttempt,
				CompletedAt:    run.Metadata.CompletedAt,
				RunArchivePath: runArchivePath(run.Metadata),
				Number:         reviewed.Number,
				Title:          reviewed.Title,
				URL:            reviewed.URL,
				HeadSHA:        reviewed.HeadSHA,
				Comments:       slices.Clone(reviewed.PublishedComments),
			}
			index.Entries = appendOrReplacePublishedEntry(index.Entries, entry)
		}
	}
	slices.SortFunc(index.Entries, func(left, right PublishedReviewEntry) int {
		if compared := right.CompletedAt.Compare(left.CompletedAt); compared != 0 {
			return compared
		}
		if left.RunID != right.RunID {
			return strings.Compare(right.RunID, left.RunID)
		}
		if left.RunAttempt != right.RunAttempt {
			return right.RunAttempt - left.RunAttempt
		}
		return right.Number - left.Number
	})
	if len(index.Entries) > maxPublishedReviewEntries {
		index.Entries = index.Entries[:maxPublishedReviewEntries]
	}
	content, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode published review index: %w", err)
	}
	content = append(content, '\n')
	for _, path := range []string{jsonPath, markdownPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create published review index directory: %w", err)
		}
	}
	if err := writeFileAtomic(jsonPath, content); err != nil {
		return fmt.Errorf("write published review index: %w", err)
	}
	if err := writeFileAtomic(markdownPath, []byte(formatPublishedReviewIndex(index))); err != nil {
		return fmt.Errorf("write published review index Markdown: %w", err)
	}
	return nil
}

func WritePublishedCommentsFile(directory string, record PollRunRecord) (string, error) {
	count := 0
	for _, reviewed := range record.Reviews {
		count += len(reviewed.PublishedComments)
	}
	path := filepath.Join(directory, fmt.Sprintf("published-comments-%d.md", count))
	var output strings.Builder
	fmt.Fprintf(&output, "# Published comments: %d\n\n", count)
	if count == 0 {
		output.WriteString("This run did not publish any review comments.\n")
	} else {
		for _, reviewed := range record.Reviews {
			for _, comment := range reviewed.PublishedComments {
				fmt.Fprintf(
					&output,
					"## [#%d — %s](%s) at `%s:%d`\n\n%s\n\n",
					reviewed.Number,
					review.SanitizeMarkdownText(reviewed.Title),
					reviewed.URL,
					review.SanitizeMarkdownText(comment.Path),
					comment.Line,
					comment.Body,
				)
			}
		}
	}
	if err := writeFileAtomic(path, []byte(output.String())); err != nil {
		return "", fmt.Errorf("write published comments summary: %w", err)
	}
	return path, nil
}

func loadPollRunRecord(path string) (PollRunRecord, error) {
	var record PollRunRecord
	if err := decodeStrictJSONFile(path, &record, maxSavedReportBytes); err != nil {
		return record, fmt.Errorf("load poll run record: %w", err)
	}
	if record.Version != runRecordVersion {
		return record, fmt.Errorf("unsupported poll run record version %d", record.Version)
	}
	return record, nil
}

func loadPublishedReviewIndex(path string) (PublishedReviewIndex, error) {
	index := PublishedReviewIndex{Version: publishedReviewIndexVersion}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return index, nil
	} else if err != nil {
		return index, fmt.Errorf("stat published review index: %w", err)
	}
	if err := decodeStrictJSONFile(path, &index, maxPublishedReviewIndexBytes); err != nil {
		return index, fmt.Errorf("load published review index: %w", err)
	}
	if index.Version != publishedReviewIndexVersion {
		return index, fmt.Errorf("unsupported published review index version %d", index.Version)
	}
	return index, nil
}

type strictJSONTarget interface {
	PollRunRecord | PublishedReviewIndex | outcomes.Ledger
}

func decodeStrictJSONFile[T strictJSONTarget](path string, target *T, maxBytes int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return fmt.Errorf("%q must be a regular file no larger than %d bytes", path, maxBytes)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%q contains trailing JSON", path)
	}
	return nil
}

func appendOrReplacePublishedEntry(entries []PublishedReviewEntry, entry PublishedReviewEntry) []PublishedReviewEntry {
	for index, existing := range entries {
		if existing.RunID == entry.RunID &&
			existing.RunAttempt == entry.RunAttempt &&
			existing.Number == entry.Number &&
			existing.HeadSHA == entry.HeadSHA {
			entries[index] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func runArchivePath(metadata PollRunMetadata) string {
	return fmt.Sprintf(
		"runs/%s/%s-%d/run.md",
		metadata.CompletedAt.UTC().Format("2006/01/02"),
		metadata.RunID,
		metadata.RunAttempt,
	)
}

func formatPublishedReviewIndex(index PublishedReviewIndex) string {
	var output strings.Builder
	output.WriteString("# Published performance reviews\n\n")
	fmt.Fprintf(
		&output,
		"Newest publications are listed first. This index retains the newest %d publications; complete history remains in the run archive.\n",
		maxPublishedReviewEntries,
	)
	for _, entry := range index.Entries {
		fmt.Fprintf(
			&output,
			"\n## [#%d — %s](%s)\n\n- Published comments: **%d**\n- Head: `%s`\n- Completed: `%s`\n- Run: [%s attempt %d](%s)\n",
			entry.Number,
			review.SanitizeMarkdownText(entry.Title),
			entry.URL,
			len(entry.Comments),
			review.SanitizeMarkdownText(entry.HeadSHA),
			entry.CompletedAt.UTC().Format(time.RFC3339),
			review.SanitizeMarkdownText(entry.RunID),
			entry.RunAttempt,
			entry.RunArchivePath,
		)
		for _, comment := range entry.Comments {
			fmt.Fprintf(
				&output,
				"\n### `%s:%d`\n\n%s\n",
				review.SanitizeMarkdownText(comment.Path),
				comment.Line,
				comment.Body,
			)
		}
	}
	if len(index.Entries) == 0 {
		output.WriteString("\nNo automatic review comments have been published yet.\n")
	}
	return output.String()
}
