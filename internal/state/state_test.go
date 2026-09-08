package state

import (
	"context"
	"encoding/json"
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

	if err := store.Save(context.Background(), New(), ""); err != nil {
		t.Fatal(err)
	}
	if !client.ensuredBranch || client.savedSHA != "" {
		t.Fatalf("ensured=%v saved SHA=%q", client.ensuredBranch, client.savedSHA)
	}
	var saved State
	if err := json.Unmarshal(client.saved, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != CurrentVersion || saved.PullRequests == nil {
		t.Fatalf("saved state = %+v", saved)
	}
}

func TestStoreMigratesVersionOneByRequiringBootstrap(t *testing.T) {
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
	if !exists || sha != "content-sha" || loaded.Version != CurrentVersion || loaded.Initialized {
		t.Fatalf("loaded=%+v exists=%v sha=%q", loaded, exists, sha)
	}
}

func TestStoreMigratesVersionTwoStateToReviewedPullRequests(t *testing.T) {
	client := &fakeContentClient{
		exists: true,
		content: github.FileContent{
			SHA: "content-sha",
			Content: []byte(`{
				"version":2,
				"initialized":true,
				"pullRequests":{
					"7":{
						"reviewedHeadSha":"head",
						"pendingHeadSha":"new-head",
						"pendingSince":"2026-08-24T11:00:00Z",
						"state":"open",
						"draft":false
					},
					"8":{"pendingHeadSha":"pending","state":"open","draft":false}
				},
				"daily":{"date":"2026-08-24","reviews":3,"publications":1}
			}`),
		},
	}
	store := NewStore(client, "owner", "repo", "reviewer-state", "state.json")

	loaded, _, _, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Initialized || !loaded.PullRequests[7].Reviewed ||
		loaded.PullRequests[7].ReviewedHeadSHA != "head" ||
		loaded.PullRequests[7].PendingHeadSHA != "" || loaded.PullRequests[8].Reviewed ||
		loaded.Daily.Reviews != 3 || loaded.Version != CurrentVersion {
		t.Fatalf("loaded state = %+v", loaded)
	}
}
