package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/poller"
	"github.com/roblourens/rob-reviewer/internal/review"
)

type runPublisherClient struct {
	pulls    map[int]review.PullRequest
	requests map[int]review.ReviewRequest
	markers  map[int]*review.ExistingReview
	created  int
	failPR   int
}

func (client *runPublisherClient) GetPullRequest(_ context.Context, _, _ string, number int) (review.PullRequest, error) {
	return client.pulls[number], nil
}

func (client *runPublisherClient) FindReviewMarker(_ context.Context, _, _ string, number int, _ string) (*review.ExistingReview, error) {
	return client.markers[number], nil
}

func (client *runPublisherClient) FindPendingReview(_ context.Context, _, _ string, number int) (*review.ExistingReview, error) {
	marker := client.markers[number]
	if marker != nil && marker.State == "PENDING" {
		return marker, nil
	}
	return nil, nil
}

func (client *runPublisherClient) GetReviewComments(_ context.Context, _, _ string, number int, _ int64) ([]review.Comment, error) {
	return client.requests[number].Comments, nil
}

func (client *runPublisherClient) CreatePendingReview(_ context.Context, _, _ string, number int, request review.ReviewRequest) (int64, error) {
	client.created++
	client.requests[number] = request
	client.markers[number] = &review.ExistingReview{ID: int64(number), State: "PENDING", Body: request.Body}
	return int64(number), nil
}

func (client *runPublisherClient) SubmitPendingReview(_ context.Context, _, _ string, number int, _ int64) error {
	if number == client.failPR {
		return errors.New("publication unavailable")
	}
	client.markers[number].State = "COMMENTED"
	return nil
}

func (client *runPublisherClient) DeletePendingReview(_ context.Context, _, _ string, number int, _ int64) error {
	delete(client.markers, number)
	return nil
}

func publicationFixture(t *testing.T, results ...review.Result) (string, PollRunMetadata, *SavedReportPublisher, *runPublisherClient) {
	t.Helper()
	directory := t.TempDir()
	metadata := PollRunMetadata{
		RunID: "123", RunAttempt: 1, Repository: "owner/reviewer", CommitSHA: "trusted-source",
		StartedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(),
	}
	var reviewed []int
	client := &runPublisherClient{
		pulls: make(map[int]review.PullRequest), requests: make(map[int]review.ReviewRequest),
		markers: make(map[int]*review.ExistingReview),
	}
	for _, result := range results {
		if _, err := WriteResultFiles(directory, result); err != nil {
			t.Fatal(err)
		}
		reviewed = append(reviewed, result.PullRequest.Number)
		client.pulls[result.PullRequest.Number] = result.PullRequest
	}
	if _, err := WritePollRunFiles(directory, PollResult{
		Poll: poller.Result{Reviewed: reviewed}, Reviews: results,
	}, nil, metadata); err != nil {
		t.Fatal(err)
	}
	cfg := configForTest()
	cfg.Automation.Enabled = true
	cfg.Poll.MaxPerRun = 5
	cfg.Publication = config.PublicationConfig{Mode: "automatic", OptIn: &config.OptInConfig{}}
	publisher := &SavedReportPublisher{
		config: cfg, client: client, check: func(context.Context, review.PullRequest) error { return nil },
	}
	return directory, metadata, publisher, client
}

func TestPublishRunPublishesDraftAndRecordsExactComments(t *testing.T) {
	result := savedResult()
	result.PullRequest.Draft = true
	directory, metadata, publisher, client := publicationFixture(t, result)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := publisher.PublishRun(context.Background(), directory, metadata, logger); err != nil {
		t.Fatal(err)
	}
	record, err := loadPollRunRecord(filepath.Join(directory, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Published) != 1 || !record.Reviews[0].Published || len(record.Reviews[0].PublishedComments) != 1 ||
		record.Reviews[0].PublishedComments[0].Body != client.requests[7].Comments[0].Body {
		t.Fatalf("publication record=%+v", record)
	}
	if _, err := os.Stat(filepath.Join(directory, "published-comments-0.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outdated zero-comment summary still exists: %v", err)
	}
	if err := publisher.PublishRun(context.Background(), directory, metadata, logger); err != nil || client.created != 1 {
		t.Fatalf("retry created=%d err=%v", client.created, err)
	}
}

func TestPublishRunHonorsOptOutAndChangedRevisions(t *testing.T) {
	for _, reason := range []string{"opt-out", "head", "base", "closed", "disabled", "no-opt-in-config"} {
		t.Run(reason, func(t *testing.T) {
			directory, metadata, publisher, client := publicationFixture(t, savedResult())
			pull := client.pulls[7]
			switch reason {
			case "opt-out":
				publisher.check = func(context.Context, review.PullRequest) error { return review.ErrPublicationSuppressed }
			case "head":
				pull.HeadSHA = "new-head"
			case "base":
				pull.BaseSHA = "new-base"
			case "closed":
				pull.State = "closed"
			case "disabled":
				publisher.config.Publication.Mode = "approval"
			case "no-opt-in-config":
				publisher.config.Publication.OptIn = nil
			}
			client.pulls[7] = pull
			err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default())
			if (err != nil) != (reason == "no-opt-in-config") || client.created != 0 {
				t.Fatalf("created=%d err=%v", client.created, err)
			}
		})
	}
}

func TestPublishRunRejectsMismatchedSourceAndTarget(t *testing.T) {
	for _, field := range []string{"run", "attempt", "repository", "commit", "target"} {
		t.Run(field, func(t *testing.T) {
			directory, metadata, publisher, client := publicationFixture(t, savedResult())
			switch field {
			case "run":
				metadata.RunID = "another-run"
			case "attempt":
				metadata.RunAttempt++
			case "repository":
				metadata.Repository = "another/reviewer"
			case "commit":
				metadata.CommitSHA = "another-commit"
			case "target":
				publisher.config.Target.Repo = "another-repository"
			}
			if err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default()); err == nil || client.created != 0 {
				t.Fatalf("created=%d err=%v", client.created, err)
			}
		})
	}
}

func TestPublishRunDoesNotPublishReplaysOrIncompleteAnalysis(t *testing.T) {
	result := savedResult()
	directory, metadata, publisher, client := publicationFixture(t)
	if _, err := WriteResultFiles(directory, result); err != nil {
		t.Fatal(err)
	}
	if err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default()); err != nil || client.created != 0 {
		t.Fatalf("replay publication: created=%d err=%v", client.created, err)
	}
	directory, metadata, publisher, client = publicationFixture(t, result)
	if _, err := WritePollRunFiles(directory, PollResult{Reviews: []review.Result{result}}, errors.New("analysis aborted"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default()); err != nil || client.created != 0 {
		t.Fatalf("incomplete publication: created=%d err=%v", client.created, err)
	}
}

func TestPublishRunPreservesPartialPublicationAndResumesPending(t *testing.T) {
	first := savedResult()
	second := savedResult()
	second.PullRequest.Number = 8
	second.PullRequest.URL = "https://github.com/microsoft/vscode/pull/8"
	review.BindFindingIDs(&second)
	directory, metadata, publisher, client := publicationFixture(t, first, second)
	client.failPR = 8
	if err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default()); err == nil {
		t.Fatal("expected second publication to fail")
	}
	record, err := loadPollRunRecord(filepath.Join(directory, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Published) != 1 || !record.Reviews[0].Published || record.Reviews[1].Published || record.Succeeded {
		t.Fatalf("partial record=%+v", record)
	}
	client.failPR = 0
	if err := publisher.PublishRun(context.Background(), directory, metadata, slog.Default()); err != nil || client.created != 2 {
		t.Fatalf("pending retry created=%d err=%v", client.created, err)
	}
	record, err = loadPollRunRecord(filepath.Join(directory, "run.json"))
	if err != nil || len(record.Published) != 2 || !record.Reviews[1].Published || !record.Succeeded || record.PublicationError != "" {
		t.Fatalf("resumed record=%+v err=%v", record, err)
	}
}
