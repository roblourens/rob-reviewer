package state

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/roblourens/rob-reviewer/internal/github"
)

const CurrentVersion = 1

type State struct {
	Version            int       `json:"version"`
	HighWaterMark      int       `json:"highWaterMark"`
	HighWaterCreatedAt time.Time `json:"highWaterCreatedAt"`
	PendingDrafts      []int     `json:"pendingDrafts"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func New(highWaterMark int) State {
	return State{
		Version:       CurrentVersion,
		HighWaterMark: highWaterMark,
		PendingDrafts: []int{},
		UpdatedAt:     time.Now().UTC(),
	}
}

func (state *State) AddPendingDraft(number int) {
	if !slices.Contains(state.PendingDrafts, number) {
		state.PendingDrafts = append(state.PendingDrafts, number)
		slices.Sort(state.PendingDrafts)
	}
}

func (state *State) RemovePendingDraft(number int) {
	state.PendingDrafts = slices.DeleteFunc(state.PendingDrafts, func(candidate int) bool {
		return candidate == number
	})
}

type ContentClient interface {
	EnsureBranch(ctx context.Context, owner, repo, branch string) error
	GetContent(ctx context.Context, owner, repo, path, branch string) (github.FileContent, bool, error)
	PutContent(ctx context.Context, owner, repo, path, branch, message string, content []byte, sha string) error
}

type Store struct {
	client ContentClient
	owner  string
	repo   string
	branch string
	path   string
}

func NewStore(client ContentClient, owner, repo, branch, path string) *Store {
	return &Store{
		client: client,
		owner:  owner,
		repo:   repo,
		branch: branch,
		path:   path,
	}
}

func (store *Store) Load(ctx context.Context) (State, string, bool, error) {
	content, exists, err := store.client.GetContent(ctx, store.owner, store.repo, store.path, store.branch)
	if err != nil {
		return State{}, "", false, fmt.Errorf("load reviewer state: %w", err)
	}
	if !exists {
		return State{}, "", false, nil
	}
	var result State
	if err := json.Unmarshal(content.Content, &result); err != nil {
		return State{}, "", false, fmt.Errorf("decode reviewer state: %w", err)
	}
	if result.Version != CurrentVersion {
		return State{}, "", false, fmt.Errorf("unsupported reviewer state version %d", result.Version)
	}
	slices.Sort(result.PendingDrafts)
	result.PendingDrafts = slices.Compact(result.PendingDrafts)
	return result, content.SHA, true, nil
}

func (store *Store) Save(ctx context.Context, state State, sha string) error {
	state.Version = CurrentVersion
	state.UpdatedAt = time.Now().UTC()
	slices.Sort(state.PendingDrafts)
	state.PendingDrafts = slices.Compact(state.PendingDrafts)
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode reviewer state: %w", err)
	}
	content = append(content, '\n')

	if sha == "" {
		if err := store.client.EnsureBranch(ctx, store.owner, store.repo, store.branch); err != nil {
			return fmt.Errorf("ensure reviewer state branch: %w", err)
		}
	}
	if err := store.client.PutContent(
		ctx,
		store.owner,
		store.repo,
		store.path,
		store.branch,
		"Update reviewer state (Written by Copilot)",
		content,
		sha,
	); err != nil {
		return fmt.Errorf("save reviewer state: %w", err)
	}
	return nil
}
