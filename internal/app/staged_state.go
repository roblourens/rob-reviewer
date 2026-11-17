package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/roblourens/rob-reviewer/internal/config"
	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/poller"
	"github.com/roblourens/rob-reviewer/internal/state"
)

type stagedStateUpdate struct {
	SHA   string      `json:"sha"`
	State state.State `json:"state"`
}

type stagedStateStore struct {
	source poller.StateStore
	path   string
}

func (store *stagedStateStore) Load(ctx context.Context) (state.State, string, bool, error) {
	return store.source.Load(ctx)
}

func (store *stagedStateStore) Save(_ context.Context, current state.State, sha string) error {
	current.UpdatedAt = time.Now().UTC()
	content, err := json.MarshalIndent(stagedStateUpdate{SHA: sha, State: current}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode staged state update: %w", err)
	}
	if err := writeFileAtomic(store.path, append(content, '\n')); err != nil {
		return fmt.Errorf("stage state update: %w", err)
	}
	return nil
}

func ApplyStateUpdate(ctx context.Context, configPath, repository, path, token string) error {
	if token == "" {
		return errors.New("GITHUB_TOKEN is required to persist staged reviewer state")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	owner, repo, err := parseRepository(repository)
	if err != nil {
		return err
	}
	store := state.NewStore(github.NewClient(nil, token), owner, repo, cfg.State.Branch, cfg.State.Path)
	return applyStateUpdate(ctx, path, store)
}

func applyStateUpdate(ctx context.Context, path string, store poller.StateStore) error {
	var update stagedStateUpdate
	if err := decodeStrictJSONFile(path, &update, maxSavedReportBytes); err != nil {
		return fmt.Errorf("load staged state update: %w", err)
	}
	if update.State.Version != state.CurrentVersion || !update.State.Initialized || update.State.PullRequests == nil {
		return errors.New("staged state update is missing initialized current-version state")
	}
	return store.Save(ctx, update.State, update.SHA)
}
