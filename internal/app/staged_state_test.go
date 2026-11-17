package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/state"
)

type stateStoreSpy struct {
	current state.State
	sha     string
	saves   int
	saveErr error
}

func (store *stateStoreSpy) Load(context.Context) (state.State, string, bool, error) {
	return store.current, store.sha, true, nil
}

func (store *stateStoreSpy) Save(_ context.Context, current state.State, sha string) error {
	store.saves++
	store.current, store.sha = current, sha
	return store.saveErr
}

func TestStagedStateDefersWritesAndPreservesOptimisticSHA(t *testing.T) {
	current := state.New()
	current.Initialized = true
	current.PullRequests[7] = state.PullRequestState{Reviewed: true, ReviewedHeadSHA: "head"}
	source := &stateStoreSpy{current: current, sha: "original-sha"}
	path := filepath.Join(t.TempDir(), "state-update.json")
	staged := &stagedStateStore{source: source, path: path}
	loaded, sha, exists, err := staged.Load(context.Background())
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := staged.Save(ctx, loaded, sha); err != nil {
		t.Fatal(err)
	}
	if source.saves != 0 {
		t.Fatal("analysis persisted state remotely")
	}
	target := &stateStoreSpy{}
	if err := applyStateUpdate(context.Background(), path, target); err != nil {
		t.Fatal(err)
	}
	if target.saves != 1 || target.sha != "original-sha" || !target.current.PullRequests[7].Reviewed {
		t.Fatalf("applied state=%+v", target)
	}
	conflict := errors.New("state changed concurrently")
	target.saveErr = conflict
	if err := applyStateUpdate(context.Background(), path, target); !errors.Is(err, conflict) {
		t.Fatalf("lost optimistic update failure: %v", err)
	}
}

func TestApplyStagedStateRejectsInvalidOrInjectedTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state-update.json")
	for _, content := range []string{
		`{}`,
		`{"sha":"sha","state":{"version":3,"initialized":false,"pullRequests":{}}}`,
		`{"sha":"sha","state":{"version":99,"initialized":true,"pullRequests":{}}}`,
		`{"sha":"sha","state":{"version":3,"initialized":true,"pullRequests":{}},"repository":"another/repo"}`,
		`{"sha":"sha","state":{"version":3,"initialized":true,"pullRequests":{}}} {}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		store := &stateStoreSpy{}
		if err := applyStateUpdate(context.Background(), path, store); err == nil || store.saves != 0 {
			t.Fatalf("saves=%d err=%v", store.saves, err)
		}
	}
}
