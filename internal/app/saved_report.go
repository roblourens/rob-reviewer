package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/review"
)

const maxSavedReportBytes = 20 << 20

type SavedReportPublisher struct {
	config config.Config
	client review.PublisherClient
}

func NewSavedReportPublisher(configPath, reviewToken string) (*SavedReportPublisher, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reviewToken) == "" {
		return nil, errors.New("REVIEW_GITHUB_TOKEN is required")
	}
	return &SavedReportPublisher{
		config: cfg,
		client: github.NewClient(nil, reviewToken),
	}, nil
}

func (publisher *SavedReportPublisher) Publish(
	ctx context.Context,
	reportPath string,
	approvedIDs []string,
) (review.Result, bool, error) {
	result, err := LoadResult(reportPath)
	if err != nil {
		return review.Result{}, false, err
	}
	selected, err := review.SelectFindings(result, approvedIDs)
	if err != nil {
		return review.Result{}, false, err
	}
	published, err := review.NewPublisher(
		publisher.client,
		publisher.config.Target.Owner,
		publisher.config.Target.Repo,
		publisher.config.Automation.SkipLabels...,
	).Publish(ctx, selected, false)
	if err != nil {
		return review.Result{}, false, err
	}
	return selected, published, nil
}

func LoadResult(path string) (review.Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return review.Result{}, fmt.Errorf("open saved review report %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return review.Result{}, fmt.Errorf("stat saved review report %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return review.Result{}, fmt.Errorf("saved review report %q is not a regular file", path)
	}
	if info.Size() > maxSavedReportBytes {
		return review.Result{}, fmt.Errorf("saved review report %q exceeds %d bytes", path, maxSavedReportBytes)
	}
	var result review.Result
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return review.Result{}, fmt.Errorf("decode saved review report %q: %w", path, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return review.Result{}, fmt.Errorf("saved review report %q contains multiple JSON values", path)
		}
		return review.Result{}, fmt.Errorf("read saved review report %q: %w", path, err)
	}
	if result.PullRequest.Number < 1 || result.PullRequest.BaseSHA == "" || result.PullRequest.HeadSHA == "" {
		return review.Result{}, fmt.Errorf("saved review report %q is missing pull request identity", path)
	}
	if err := review.ValidateFindingIDs(result); err != nil {
		return review.Result{}, fmt.Errorf("validate saved review report %q: %w", path, err)
	}
	return result, nil
}

func WriteResultFiles(directory string, result review.Result) ([]string, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("output directory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create review output directory: %w", err)
	}
	stem := fmt.Sprintf("pr-%d-%s", result.PullRequest.Number, shortSHA(result.PullRequest.HeadSHA))
	jsonPath := filepath.Join(directory, stem+".json")
	markdownPath := filepath.Join(directory, stem+".md")
	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode review result: %w", err)
	}
	content = append(content, '\n')
	if err := writeFileAtomic(jsonPath, content); err != nil {
		return nil, fmt.Errorf("write JSON review result: %w", err)
	}
	if err := writeFileAtomic(markdownPath, []byte(FormatResultMarkdown(result))); err != nil {
		return nil, fmt.Errorf("write Markdown review result: %w", err)
	}
	return []string{jsonPath, markdownPath}, nil
}

func writeFileAtomic(path string, content []byte) (returnErr error) {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() {
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
