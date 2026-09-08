package poller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
	"github.com/roblourens/rob-reviewer/internal/state"
)

type PullRequestClient interface {
	ListOpenPullRequests(ctx context.Context, owner, repo string, page int) ([]review.PullRequest, bool, error)
	ListUpdatedPullRequests(ctx context.Context, owner, repo string, page int) ([]review.PullRequest, bool, error)
	GetPullRequest(ctx context.Context, owner, repo string, number int) (review.PullRequest, error)
}

type StateStore interface {
	Load(ctx context.Context) (state.State, string, bool, error)
	Save(ctx context.Context, state state.State, sha string) error
}

type ReviewOutcome struct {
	Published bool
}

type Reviewer func(context.Context, review.PullRequest) (ReviewOutcome, error)

type Options struct {
	Owner         string
	Repo          string
	MaxPerRun     int
	MaxPerDay     int
	QuietPeriod   time.Duration
	ScanWindow    time.Duration
	MaxPendingAge time.Duration
	SkipLabels    []string
	Now           func() time.Time
}

type Poller struct {
	client  PullRequestClient
	store   StateStore
	options Options
	review  Reviewer
}

type Result struct {
	Bootstrapped int
	Reviewed     []int
	Published    []int
	Deferred     []int
	Skipped      []int
}

func New(client PullRequestClient, store StateStore, options Options, reviewer Reviewer) *Poller {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Poller{client: client, store: store, options: options, review: reviewer}
}

func (poller *Poller) Run(ctx context.Context) (result Result, returnErr error) {
	current, sha, exists, err := poller.store.Load(ctx)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		current = state.New()
	}
	if !current.Initialized {
		count, err := poller.bootstrap(ctx, &current)
		if err != nil {
			return Result{}, err
		}
		if err := poller.store.Save(ctx, current, sha); err != nil {
			return Result{}, err
		}
		return Result{Bootstrapped: count}, nil
	}

	dirty := false
	defer func() {
		if !dirty {
			return
		}
		saveContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := poller.store.Save(saveContext, current, sha); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	now := poller.options.Now().UTC()
	current.ResetDaily(now)
	dirty = true
	updated, err := poller.updatedSince(ctx, now.Add(-poller.options.ScanWindow))
	if err != nil {
		return result, err
	}
	seen := make(map[int]struct{}, len(updated))
	for _, pull := range updated {
		seen[pull.Number] = struct{}{}
		poller.observe(&current, pull, now, &result)
	}

	for number, tracked := range current.PullRequests {
		if tracked.PendingHeadSHA == "" {
			continue
		}
		if _, exists := seen[number]; exists {
			continue
		}
		pull, err := poller.client.GetPullRequest(ctx, poller.options.Owner, poller.options.Repo, number)
		if err != nil {
			return result, fmt.Errorf("refresh pending PR %d: %w", number, err)
		}
		poller.observe(&current, pull, now, &result)
	}

	for number, tracked := range current.PullRequests {
		if tracked.PendingHeadSHA == "" || tracked.PendingSince.IsZero() ||
			now.Sub(tracked.PendingSince) <= poller.options.MaxPendingAge {
			continue
		}
		tracked.ReviewedHeadSHA = tracked.PendingHeadSHA
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
		current.PullRequests[number] = tracked
		appendUnique(&result.Skipped, number)
	}

	candidates := poller.readyCandidates(current, now)
	remainingToday := max(0, poller.options.MaxPerDay-current.Daily.Reviews)
	limit := min(poller.options.MaxPerRun, remainingToday)
	for index, candidate := range candidates {
		if index >= limit {
			appendUnique(&result.Deferred, candidate.number)
			continue
		}
		pull, err := poller.client.GetPullRequest(ctx, poller.options.Owner, poller.options.Repo, candidate.number)
		if err != nil {
			return result, fmt.Errorf("refresh PR %d before review: %w", candidate.number, err)
		}
		if !poller.stillReady(&current, pull, now, &result) {
			continue
		}
		outcome, err := poller.review(ctx, pull)
		if err != nil {
			return result, fmt.Errorf("review PR %d at %s: %w", pull.Number, pull.HeadSHA, err)
		}
		tracked := current.PullRequests[pull.Number]
		tracked.Reviewed = true
		tracked.ReviewedHeadSHA = pull.HeadSHA
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
		tracked.State = pull.State
		tracked.Draft = pull.Draft
		tracked.UpdatedAt = pull.UpdatedAt
		current.PullRequests[pull.Number] = tracked
		current.Daily.Reviews++
		result.Reviewed = append(result.Reviewed, pull.Number)
		if outcome.Published {
			current.Daily.Publications++
			result.Published = append(result.Published, pull.Number)
		}
	}

	boundary := now.Add(-poller.options.ScanWindow)
	for number, tracked := range current.PullRequests {
		if !tracked.Reviewed && strings.EqualFold(tracked.State, "closed") &&
			tracked.UpdatedAt.Before(boundary) && tracked.PendingHeadSHA == "" {
			delete(current.PullRequests, number)
		}
	}
	return result, nil
}

func (poller *Poller) bootstrap(ctx context.Context, current *state.State) (int, error) {
	count := 0
	for page := 1; ; page++ {
		pulls, hasNext, err := poller.client.ListOpenPullRequests(ctx, poller.options.Owner, poller.options.Repo, page)
		if err != nil {
			return 0, fmt.Errorf("list open pull requests page %d for bootstrap: %w", page, err)
		}
		for _, pull := range pulls {
			if !pull.IsTeamAuthored() {
				continue
			}
			tracked := state.PullRequestState{
				State:     pull.State,
				Draft:     pull.Draft,
				UpdatedAt: pull.UpdatedAt,
			}
			if pull.Draft {
				tracked.PendingHeadSHA = pull.HeadSHA
			} else {
				tracked.ReviewedHeadSHA = pull.HeadSHA
			}
			current.PullRequests[pull.Number] = tracked
			count++
		}
		if !hasNext {
			break
		}
	}
	current.Initialized = true
	current.ResetDaily(poller.options.Now())
	return count, nil
}

func (poller *Poller) updatedSince(ctx context.Context, boundary time.Time) ([]review.PullRequest, error) {
	var result []review.PullRequest
	for page := 1; ; page++ {
		pulls, hasNext, err := poller.client.ListUpdatedPullRequests(ctx, poller.options.Owner, poller.options.Repo, page)
		if err != nil {
			return nil, fmt.Errorf("list updated pull requests page %d: %w", page, err)
		}
		reachedBoundary := false
		for _, pull := range pulls {
			if !pull.UpdatedAt.IsZero() && pull.UpdatedAt.Before(boundary) {
				reachedBoundary = true
				continue
			}
			result = append(result, pull)
		}
		if reachedBoundary || !hasNext {
			break
		}
	}
	slices.SortFunc(result, func(left, right review.PullRequest) int {
		if compared := left.UpdatedAt.Compare(right.UpdatedAt); compared != 0 {
			return compared
		}
		return left.Number - right.Number
	})
	return result, nil
}

func (poller *Poller) observe(current *state.State, pull review.PullRequest, now time.Time, result *Result) {
	tracked, existed := current.PullRequests[pull.Number]
	previousState := tracked.State
	previousDraft := tracked.Draft
	tracked.State = pull.State
	tracked.Draft = pull.Draft
	tracked.UpdatedAt = pull.UpdatedAt

	switch {
	case tracked.Reviewed:
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
	case !pull.IsTeamAuthored():
		tracked.ReviewedHeadSHA = pull.HeadSHA
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
		appendUnique(&result.Skipped, pull.Number)
	case pull.HasAnyLabel(poller.options.SkipLabels):
		tracked.ReviewedHeadSHA = pull.HeadSHA
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
		appendUnique(&result.Skipped, pull.Number)
	case !strings.EqualFold(pull.State, "open"):
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
	case pull.Draft:
		if tracked.PendingHeadSHA != pull.HeadSHA {
			tracked.PendingHeadSHA = pull.HeadSHA
			tracked.PendingSince = time.Time{}
		}
		appendUnique(&result.Deferred, pull.Number)
	case tracked.ReviewedHeadSHA == pull.HeadSHA && tracked.PendingHeadSHA == "" &&
		(!existed || (strings.EqualFold(previousState, "open") && !previousDraft)):
		tracked.PendingHeadSHA = ""
		tracked.PendingSince = time.Time{}
	default:
		if tracked.PendingHeadSHA != pull.HeadSHA || previousDraft || !strings.EqualFold(previousState, "open") {
			tracked.PendingHeadSHA = pull.HeadSHA
			tracked.PendingSince = now
		}
		if now.Sub(tracked.PendingSince) < poller.options.QuietPeriod {
			appendUnique(&result.Deferred, pull.Number)
		}
	}
	current.PullRequests[pull.Number] = tracked
}

type candidate struct {
	number       int
	pendingSince time.Time
}

func (poller *Poller) readyCandidates(current state.State, now time.Time) []candidate {
	var result []candidate
	for number, tracked := range current.PullRequests {
		if tracked.Reviewed || tracked.PendingHeadSHA == "" || tracked.PendingSince.IsZero() ||
			tracked.Draft || !strings.EqualFold(tracked.State, "open") {
			continue
		}
		if now.Sub(tracked.PendingSince) >= poller.options.QuietPeriod {
			result = append(result, candidate{number: number, pendingSince: tracked.PendingSince})
		}
	}
	slices.SortFunc(result, func(left, right candidate) int {
		if compared := left.pendingSince.Compare(right.pendingSince); compared != 0 {
			return compared
		}
		return left.number - right.number
	})
	return result
}

func (poller *Poller) stillReady(current *state.State, pull review.PullRequest, now time.Time, result *Result) bool {
	tracked := current.PullRequests[pull.Number]
	if tracked.Reviewed || !pull.IsTeamAuthored() || pull.HasAnyLabel(poller.options.SkipLabels) ||
		!strings.EqualFold(pull.State, "open") || pull.Draft || pull.HeadSHA != tracked.PendingHeadSHA {
		poller.observe(current, pull, now, result)
		return false
	}
	return true
}

func appendUnique(values *[]int, value int) {
	if !slices.Contains(*values, value) {
		*values = append(*values, value)
	}
}
