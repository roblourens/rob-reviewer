package poller

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
	"github.com/roblourens/rob-reviewer/internal/state"
)

type fakePullClient struct {
	openPages    map[int][]review.PullRequest
	updatedPages map[int][]review.PullRequest
	pulls        map[int]review.PullRequest
}

func (client *fakePullClient) ListOpenPullRequests(_ context.Context, _, _ string, page int) ([]review.PullRequest, bool, error) {
	pulls := slices.Clone(client.openPages[page])
	_, next := client.openPages[page+1]
	return pulls, next, nil
}

func (client *fakePullClient) ListUpdatedPullRequests(_ context.Context, _, _ string, page int) ([]review.PullRequest, bool, error) {
	pulls := slices.Clone(client.updatedPages[page])
	_, next := client.updatedPages[page+1]
	return pulls, next, nil
}

func (client *fakePullClient) GetPullRequest(_ context.Context, _, _ string, number int) (review.PullRequest, error) {
	return client.pulls[number], nil
}

type fakeStateStore struct {
	current        state.State
	exists         bool
	saves          []state.State
	saveContextErr error
}

func (store *fakeStateStore) Load(context.Context) (state.State, string, bool, error) {
	return store.current, "state-sha", store.exists, nil
}

func (store *fakeStateStore) Save(ctx context.Context, current state.State, _ string) error {
	store.saveContextErr = ctx.Err()
	store.current = current
	store.exists = true
	store.saves = append(store.saves, current)
	return nil
}

func testOptions(now *time.Time) Options {
	return Options{
		Owner:         "microsoft",
		Repo:          "vscode",
		MaxPerRun:     5,
		MaxPerDay:     25,
		QuietPeriod:   5 * time.Minute,
		ScanWindow:    7 * 24 * time.Hour,
		MaxPendingAge: 24 * time.Hour,
		SkipLabels:    []string{"performance-reviewer:skip"},
		Now:           func() time.Time { return *now },
	}
}

func TestRunSkipsSuppressedAndExpiredHeads(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := initializedState()
	current.PullRequests[11] = state.PullRequestState{
		PendingHeadSHA: "expired",
		PendingSince:   now.Add(-25 * time.Hour),
		State:          "open",
	}
	suppressed := pull(12, "suppressed", now)
	suppressed.Labels = []string{"PERFORMANCE-REVIEWER:SKIP"}
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {suppressed}},
		pulls: map[int]review.PullRequest{
			11: pull(11, "expired", now),
			12: suppressed,
		},
	}
	store := &fakeStateStore{exists: true, current: current}
	reviewCalls := 0
	poller := New(client, store, testOptions(&now), func(context.Context, review.PullRequest) (ReviewOutcome, error) {
		reviewCalls++
		return ReviewOutcome{}, nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 0 || !slices.Equal(result.Skipped, []int{12, 11}) {
		t.Fatalf("calls=%d result=%+v", reviewCalls, result)
	}
	if store.current.PullRequests[11].ReviewedHeadSHA != "expired" ||
		store.current.PullRequests[12].ReviewedHeadSHA != "suppressed" {
		t.Fatalf("state=%+v", store.current)
	}
}

func TestRunHonorsSuppressionAddedImmediatelyBeforeReview(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := initializedState()
	current.PullRequests[12] = state.PullRequestState{
		PendingHeadSHA: "head",
		PendingSince:   now.Add(-time.Hour),
		State:          "open",
	}
	scanned := pull(12, "head", now)
	refreshed := scanned
	refreshed.Labels = []string{"performance-reviewer:skip"}
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {scanned}},
		pulls:        map[int]review.PullRequest{12: refreshed},
	}
	store := &fakeStateStore{exists: true, current: current}
	reviewCalls := 0
	poller := New(client, store, testOptions(&now), func(context.Context, review.PullRequest) (ReviewOutcome, error) {
		reviewCalls++
		return ReviewOutcome{}, nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 0 || !slices.Contains(result.Skipped, 12) ||
		store.current.PullRequests[12].ReviewedHeadSHA != "head" {
		t.Fatalf("calls=%d result=%+v state=%+v", reviewCalls, result, store.current)
	}
}

func initializedState() state.State {
	current := state.New()
	current.Initialized = true
	return current
}

func pull(number int, head string, updated time.Time) review.PullRequest {
	return review.PullRequest{
		Number:            number,
		State:             "open",
		AuthorAssociation: "MEMBER",
		HeadSHA:           head,
		UpdatedAt:         updated,
	}
}

func TestRunBootstrapsOpenHeadsWithoutReviewingBacklog(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	ready := pull(12, "ready", now)
	draft := pull(11, "draft", now)
	draft.Draft = true
	external := pull(10, "external", now)
	external.AuthorAssociation = "CONTRIBUTOR"
	client := &fakePullClient{openPages: map[int][]review.PullRequest{1: {ready, draft, external}}}
	store := &fakeStateStore{}
	reviewCalls := 0
	poller := New(client, store, testOptions(&now), func(context.Context, review.PullRequest) (ReviewOutcome, error) {
		reviewCalls++
		return ReviewOutcome{}, nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Bootstrapped != 2 || reviewCalls != 0 || !store.current.Initialized {
		t.Fatalf("result=%+v calls=%d state=%+v", result, reviewCalls, store.current)
	}
	if store.current.PullRequests[12].ReviewedHeadSHA != "ready" ||
		store.current.PullRequests[11].PendingHeadSHA != "draft" {
		t.Fatalf("state = %+v", store.current)
	}
}

func TestRunReviewsNewHeadAfterQuietPeriod(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	changed := pull(12, "new", now)
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {changed}},
		pulls:        map[int]review.PullRequest{12: changed},
	}
	store := &fakeStateStore{exists: true, current: initializedState()}
	var reviewed []string
	poller := New(client, store, testOptions(&now), func(_ context.Context, pull review.PullRequest) (ReviewOutcome, error) {
		reviewed = append(reviewed, pull.HeadSHA)
		return ReviewOutcome{Published: true}, nil
	})

	first, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewed) != 0 || !slices.Equal(first.Deferred, []int{12}) {
		t.Fatalf("first=%+v reviewed=%v", first, reviewed)
	}

	now = now.Add(6 * time.Minute)
	second, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []string{"new"}) || !slices.Equal(second.Published, []int{12}) {
		t.Fatalf("second=%+v reviewed=%v", second, reviewed)
	}
	if store.current.PullRequests[12].ReviewedHeadSHA != "new" || store.current.Daily.Reviews != 1 {
		t.Fatalf("state=%+v", store.current)
	}
}

func TestRunResetsQuietPeriodWhenHeadChanges(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	first := pull(12, "head-1", now)
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {first}},
		pulls:        map[int]review.PullRequest{12: first},
	}
	store := &fakeStateStore{exists: true, current: initializedState()}
	var reviewed []string
	poller := New(client, store, testOptions(&now), func(_ context.Context, pull review.PullRequest) (ReviewOutcome, error) {
		reviewed = append(reviewed, pull.HeadSHA)
		return ReviewOutcome{}, nil
	})
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	now = now.Add(4 * time.Minute)
	second := pull(12, "head-2", now)
	client.updatedPages[1] = []review.PullRequest{second}
	client.pulls[12] = second
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	now = now.Add(4 * time.Minute)
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reviewed) != 0 {
		t.Fatalf("reviewed too early: %v", reviewed)
	}

	now = now.Add(2 * time.Minute)
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []string{"head-2"}) {
		t.Fatalf("reviewed=%v", reviewed)
	}
}

func TestRunReviewsDraftWhenItBecomesReadyAndReopenedPull(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := initializedState()
	current.PullRequests[11] = state.PullRequestState{
		PendingHeadSHA: "draft-head",
		State:          "open",
		Draft:          true,
	}
	current.PullRequests[12] = state.PullRequestState{
		ReviewedHeadSHA: "closed-head",
		State:           "closed",
	}
	ready := pull(11, "draft-head", now)
	reopened := pull(12, "closed-head", now)
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {ready, reopened}},
		pulls: map[int]review.PullRequest{
			11: ready,
			12: reopened,
		},
	}
	store := &fakeStateStore{exists: true, current: current}
	var reviewed []int
	poller := New(client, store, testOptions(&now), func(_ context.Context, pull review.PullRequest) (ReviewOutcome, error) {
		reviewed = append(reviewed, pull.Number)
		return ReviewOutcome{}, nil
	})
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []int{11, 12}) {
		t.Fatalf("reviewed=%v", reviewed)
	}
}

func TestRunRetriesFailedPendingHeadOutsideUpdatedScan(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := initializedState()
	current.PullRequests[12] = state.PullRequestState{
		PendingHeadSHA: "head",
		PendingSince:   now.Add(-time.Hour),
		State:          "open",
	}
	target := pull(12, "head", now.Add(-8*24*time.Hour))
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {}},
		pulls:        map[int]review.PullRequest{12: target},
	}
	store := &fakeStateStore{exists: true, current: current}
	expected := errors.New("model failed")
	calls := 0
	poller := New(client, store, testOptions(&now), func(context.Context, review.PullRequest) (ReviewOutcome, error) {
		calls++
		if calls == 1 {
			return ReviewOutcome{}, expected
		}
		return ReviewOutcome{}, nil
	})

	if _, err := poller.Run(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("first error=%v", err)
	}
	if store.current.PullRequests[12].ReviewedHeadSHA != "" {
		t.Fatalf("failed head was completed: %+v", store.current.PullRequests[12])
	}
	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.current.PullRequests[12].ReviewedHeadSHA != "head" {
		t.Fatalf("retry did not complete: %+v", store.current.PullRequests[12])
	}
}

func TestRunEnforcesDailyCapAndPersistsAfterCancellation(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	current := initializedState()
	current.Daily = state.DailyState{Date: now.Format(time.DateOnly), Reviews: 1}
	for number := 11; number <= 12; number++ {
		current.PullRequests[number] = state.PullRequestState{
			PendingHeadSHA: "head",
			PendingSince:   now.Add(-time.Hour),
			State:          "open",
		}
	}
	client := &fakePullClient{
		updatedPages: map[int][]review.PullRequest{1: {}},
		pulls: map[int]review.PullRequest{
			11: pull(11, "head", now),
			12: pull(12, "head", now),
		},
	}
	store := &fakeStateStore{exists: true, current: current}
	options := testOptions(&now)
	options.MaxPerDay = 2
	ctx, cancel := context.WithCancel(context.Background())
	poller := New(client, store, options, func(context.Context, review.PullRequest) (ReviewOutcome, error) {
		cancel()
		return ReviewOutcome{}, nil
	})

	result, err := poller.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reviewed) != 1 || len(result.Deferred) != 1 || store.saveContextErr != nil {
		t.Fatalf("result=%+v saveContextErr=%v", result, store.saveContextErr)
	}
}
