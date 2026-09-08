package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/roblourens/rob-reviewer/internal/github"
)

const CurrentVersion = 3

type State struct {
	Version      int                      `json:"version"`
	Initialized  bool                     `json:"initialized"`
	PullRequests map[int]PullRequestState `json:"pullRequests"`
	Daily        DailyState               `json:"daily"`
	UpdatedAt    time.Time                `json:"updatedAt"`
}

type PullRequestState struct {
	Reviewed        bool      `json:"reviewed,omitempty"`
	ReviewedHeadSHA string    `json:"reviewedHeadSha,omitempty"`
	PendingHeadSHA  string    `json:"pendingHeadSha,omitempty"`
	PendingSince    time.Time `json:"pendingSince,omitempty"`
	State           string    `json:"state"`
	Draft           bool      `json:"draft"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type DailyState struct {
	Date         string `json:"date,omitempty"`
	Reviews      int    `json:"reviews"`
	Publications int    `json:"publications"`
}

func New() State {
	return State{
		Version:      CurrentVersion,
		PullRequests: make(map[int]PullRequestState),
		UpdatedAt:    time.Now().UTC(),
	}
}

func (state *State) ResetDaily(now time.Time) {
	date := now.UTC().Format(time.DateOnly)
	if state.Daily.Date != date {
		state.Daily = DailyState{Date: date}
	}
}

func (state *State) normalize() {
	state.Version = CurrentVersion
	if state.PullRequests == nil {
		state.PullRequests = make(map[int]PullRequestState)
	}
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
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(content.Content, &header); err != nil {
		return State{}, "", false, fmt.Errorf("decode reviewer state: %w", err)
	}
	if header.Version == 1 {
		result := New()
		return result, content.SHA, true, nil
	}
	if header.Version != 2 && header.Version != CurrentVersion {
		return State{}, "", false, fmt.Errorf("unsupported reviewer state version %d", header.Version)
	}
	var result State
	if err := json.Unmarshal(content.Content, &result); err != nil {
		return State{}, "", false, fmt.Errorf("decode reviewer state: %w", err)
	}
	if header.Version == 2 {
		for number, tracked := range result.PullRequests {
			tracked.Reviewed = tracked.ReviewedHeadSHA != ""
			if tracked.Reviewed {
				tracked.PendingHeadSHA = ""
				tracked.PendingSince = time.Time{}
			}
			result.PullRequests[number] = tracked
		}
	}
	result.normalize()
	return result, content.SHA, true, nil
}

func (store *Store) Save(ctx context.Context, state State, sha string) error {
	state.normalize()
	state.UpdatedAt = time.Now().UTC()
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
