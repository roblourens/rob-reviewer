package state

import (
	"context"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/github"
)

type fakeContentClient struct {
	content       github.FileContent
	exists        bool
	ensuredBranch bool
	saved         []byte
	savedSHA      string
}

func (client *fakeContentClient) EnsureBranch(context.Context, string, string, string) error {
	client.ensuredBranch = true
	return nil
}

func (client *fakeContentClient) GetContent(context.Context, string, string, string, string) (github.FileContent, bool, error) {
	return client.content, client.exists, nil
}

func (client *fakeContentClient) PutContent(_ context.Context, _, _, _, _, _ string, content []byte, sha string) error {
	client.saved = content
	client.savedSHA = sha
	return nil
}

func TestStoreCreatesBranchForNewState(t *testing.T) {
	client := &fakeContentClient{}
	store := NewStore(client, "owner", "repo", "reviewer-state", "state.json")

	if err := store.Save(context.Background(), New(42), ""); err != nil {
		t.Fatal(err)
	}
	if !client.ensuredBranch {
		t.Fatal("expected branch initialization")
	}
	if client.savedSHA != "" {
		t.Fatalf("saved SHA = %q, want empty", client.savedSHA)
	}
}

func TestStoreLoadsAndNormalizesPendingDrafts(t *testing.T) {
	client := &fakeContentClient{
		exists: true,
		content: github.FileContent{
			SHA:     "content-sha",
			Content: []byte(`{"version":1,"highWaterMark":42,"pendingDrafts":[9,3,9]}`),
		},
	}
	store := NewStore(client, "owner", "repo", "reviewer-state", "state.json")

	loaded, sha, exists, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !exists || sha != "content-sha" {
		t.Fatalf("exists = %v, sha = %q", exists, sha)
	}
	if len(loaded.PendingDrafts) != 2 || loaded.PendingDrafts[0] != 3 || loaded.PendingDrafts[1] != 9 {
		t.Fatalf("pending drafts = %v", loaded.PendingDrafts)
	}
}
