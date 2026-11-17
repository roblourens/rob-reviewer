package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/review"
)

type optInReaderFunc func(context.Context, string, string, string, string) (github.FileContent, bool, error)

func (read optInReaderFunc) GetContent(ctx context.Context, owner, repo, path, branch string) (github.FileContent, bool, error) {
	return read(ctx, owner, repo, path, branch)
}

func TestPublicationOptInChecksExplicitBooleanForAuthor(t *testing.T) {
	cfg := &config.OptInConfig{Owner: "owner", Repo: "private", Path: ".github/reviewer.json", Branch: "main"}
	reader := optInReaderFunc(func(_ context.Context, owner, repo, path, branch string) (github.FileContent, bool, error) {
		if owner != cfg.Owner || repo != cfg.Repo || path != cfg.Path || branch != cfg.Branch {
			t.Fatalf("unexpected opt-in location: %s/%s %s %s", owner, repo, branch, path)
		}
		return github.FileContent{Content: []byte(`{"Member":true,"other":false}`)}, true, nil
	})
	for _, test := range []struct {
		author  string
		allowed bool
	}{{"member", true}, {"MEMBER", true}, {"other", false}, {"absent", false}, {"", false}} {
		t.Run(test.author, func(t *testing.T) {
			err := publicationOptInCheck(reader, cfg)(context.Background(), review.PullRequest{AuthorLogin: test.author})
			if test.allowed && err != nil || !test.allowed && !errors.Is(err, review.ErrPublicationSuppressed) {
				t.Fatalf("allowed=%v err=%v", test.allowed, err)
			}
		})
	}
}

func TestPublicationOptInFailsClosedWithoutDisclosingContent(t *testing.T) {
	cfg := &config.OptInConfig{Owner: "owner", Repo: "private", Path: "reviewer.json", Branch: "main"}
	for _, test := range []struct {
		name    string
		content string
		exists  bool
		err     error
	}{
		{name: "missing"},
		{name: "inaccessible", err: errors.New("forbidden")},
		{name: "invalid", exists: true, content: `{"private-member":"secret-file-content"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := optInReaderFunc(func(context.Context, string, string, string, string) (github.FileContent, bool, error) {
				return github.FileContent{Content: []byte(test.content)}, test.exists, test.err
			})
			err := publicationOptInCheck(reader, cfg)(context.Background(), review.PullRequest{AuthorLogin: "member"})
			if err == nil || errors.Is(err, review.ErrPublicationSuppressed) ||
				strings.Contains(err.Error(), "private-member") || strings.Contains(err.Error(), "secret-file-content") {
				t.Fatalf("expected explicit, non-disclosing error: %v", err)
			}
		})
	}
}

func TestDecodePublicationOptInsRejectsAmbiguousValues(t *testing.T) {
	for _, content := range []string{
		`null`, `[]`, `{"member":null}`, `{"member":"true"}`, `{"member":1}`,
		`{"member":true,"MEMBER":false}`, `{"member":true,"member":true}`,
		`{" member ":true}`, `{"":true}`, `{"member":true`,
		`{"member":true} {}`, `{"member":true} trailing`, strings.Repeat(" ", (64<<10)+1),
	} {
		if _, err := decodePublicationOptIns([]byte(content)); err == nil {
			t.Fatalf("accepted invalid opt-in content of %d bytes", len(content))
		}
	}
	if optIns, err := decodePublicationOptIns([]byte(`{}`)); err != nil || len(optIns) != 0 {
		t.Fatalf("empty map: optIns=%v err=%v", optIns, err)
	}
}

func TestPublicationOptOutAfterDraftCreationPreventsSubmission(t *testing.T) {
	result := savedResult()
	result.PullRequest.AuthorLogin = "member"
	result.PullRequest.Draft = true
	client := &fakeSavedReportClient{pull: result.PullRequest}
	reads := 0
	reader := optInReaderFunc(func(context.Context, string, string, string, string) (github.FileContent, bool, error) {
		reads++
		content := `{"member":true}`
		if reads > 1 {
			content = `{"member":false}`
		}
		return github.FileContent{Content: []byte(content)}, true, nil
	})
	cfg := &config.OptInConfig{Owner: "owner", Repo: "private", Path: "reviewer.json", Branch: "main"}
	publisher := review.NewAutomaticPublisher(client, "microsoft", "vscode").WithPublicationCheck(publicationOptInCheck(reader, cfg))
	published, err := publisher.Publish(context.Background(), result, true)
	if err != nil || published || reads != 0 {
		t.Fatalf("dry run: published=%v reads=%d err=%v", published, reads, err)
	}
	published, err = publisher.Publish(context.Background(), result, false)
	if !errors.Is(err, review.ErrPublicationSuppressed) || published || reads != 2 || client.submitted != 0 || client.deleted != 42 {
		t.Fatalf("published=%v reads=%d submitted=%d deleted=%d err=%v", published, reads, client.submitted, client.deleted, err)
	}
}
