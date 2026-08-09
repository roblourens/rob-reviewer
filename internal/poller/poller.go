package poller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
	"github.com/roblourens/rob-reviewer/internal/state"
)

type PullRequestClient interface {
	ListPullRequests(ctx context.Context, owner, repo string, page int) ([]review.PullRequest, bool, error)
	GetPullRequest(ctx context.Context, owner, repo string, number int) (review.PullRequest, error)
}

type StateStore interface {
	Load(ctx context.Context) (state.State, string, bool, error)
	Save(ctx context.Context, state state.State, sha string) error
}

type Reviewer func(context.Context, review.PullRequest) error

type Poller struct {
	client    PullRequestClient
	store     StateStore
	owner     string
	repo      string
	maxPerRun int
	review    Reviewer
}

type Result struct {
	Bootstrapped int
	Reviewed     []int
	Deferred     []int
	Skipped      []int
}

func New(client PullRequestClient, store StateStore, owner, repo string, maxPerRun int, reviewer Reviewer) *Poller {
	return &Poller{
		client:    client,
		store:     store,
		owner:     owner,
		repo:      repo,
		maxPerRun: maxPerRun,
		review:    reviewer,
	}
}

func (poller *Poller) Run(ctx context.Context) (result Result, returnErr error) {
	current, sha, exists, err := poller.store.Load(ctx)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		highWater, err := poller.currentHighWaterMark(ctx)
		if err != nil {
			return Result{}, err
		}
		current = state.New(highWater.Number)
		current.HighWaterCreatedAt = highWater.CreatedAt
		if err := poller.store.Save(ctx, current, ""); err != nil {
			return Result{}, err
		}
		return Result{Bootstrapped: highWater.Number}, nil
	}

	dirty := false
	defer func() {
		if !dirty {
			return
		}
		if err := poller.store.Save(ctx, current, sha); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	reviewedCount := 0
	for _, number := range slices.Clone(current.PendingDrafts) {
		pull, err := poller.client.GetPullRequest(ctx, poller.owner, poller.repo, number)
		if err != nil {
			return result, fmt.Errorf("refresh deferred draft PR %d: %w", number, err)
		}
		switch {
		case !pull.IsTeamAuthored():
			current.RemovePendingDraft(number)
			dirty = true
			result.Skipped = append(result.Skipped, number)
		case !strings.EqualFold(pull.State, "open"):
			current.RemovePendingDraft(number)
			dirty = true
			result.Skipped = append(result.Skipped, number)
		case pull.Draft:
			result.Deferred = append(result.Deferred, number)
		case reviewedCount >= poller.maxPerRun:
			result.Deferred = append(result.Deferred, number)
		default:
			if err := poller.review(ctx, pull); err != nil {
				return result, fmt.Errorf("review deferred PR %d: %w", number, err)
			}
			current.RemovePendingDraft(number)
			dirty = true
			result.Reviewed = append(result.Reviewed, number)
			reviewedCount++
		}
	}

	newPulls, err := poller.listAfter(ctx, current)
	if err != nil {
		return result, err
	}
	for _, pull := range newPulls {
		if reviewedCount >= poller.maxPerRun {
			break
		}
		switch {
		case !pull.IsTeamAuthored():
			result.Skipped = append(result.Skipped, pull.Number)
		case !strings.EqualFold(pull.State, "open"):
			result.Skipped = append(result.Skipped, pull.Number)
		case pull.Draft:
			current.AddPendingDraft(pull.Number)
			dirty = true
			result.Deferred = append(result.Deferred, pull.Number)
		default:
			if err := poller.review(ctx, pull); err != nil {
				return result, fmt.Errorf("review PR %d: %w", pull.Number, err)
			}
			result.Reviewed = append(result.Reviewed, pull.Number)
			reviewedCount++
		}
		current.HighWaterMark = pull.Number
		current.HighWaterCreatedAt = pull.CreatedAt
		dirty = true
	}

	return result, nil
}

func (poller *Poller) currentHighWaterMark(ctx context.Context) (review.PullRequest, error) {
	pulls, _, err := poller.client.ListPullRequests(ctx, poller.owner, poller.repo, 1)
	if err != nil {
		return review.PullRequest{}, fmt.Errorf("list current pull requests for bootstrap: %w", err)
	}
	if len(pulls) == 0 {
		return review.PullRequest{}, nil
	}
	return slices.MaxFunc(pulls, func(left, right review.PullRequest) int {
		return left.Number - right.Number
	}), nil
}

func (poller *Poller) listAfter(ctx context.Context, current state.State) ([]review.PullRequest, error) {
	var result []review.PullRequest
	for page := 1; ; page++ {
		pulls, hasNext, err := poller.client.ListPullRequests(ctx, poller.owner, poller.repo, page)
		if err != nil {
			return nil, fmt.Errorf("list pull requests page %d: %w", page, err)
		}
		reachedOlderCreation := false
		pageMaximum := 0
		for _, pull := range pulls {
			pageMaximum = max(pageMaximum, pull.Number)
			if pull.Number > current.HighWaterMark {
				result = append(result, pull)
			}
			if !current.HighWaterCreatedAt.IsZero() && pull.CreatedAt.Before(current.HighWaterCreatedAt) {
				reachedOlderCreation = true
			}
		}
		reachedLegacyBoundary := current.HighWaterCreatedAt.IsZero() && pageMaximum <= current.HighWaterMark
		if reachedOlderCreation || reachedLegacyBoundary || !hasNext {
			break
		}
	}
	slices.SortFunc(result, func(left, right review.PullRequest) int {
		return left.Number - right.Number
	})
	return result, nil
}
