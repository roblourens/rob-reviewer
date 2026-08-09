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
	pages     map[int][]review.PullRequest
	pulls     map[int]review.PullRequest
	listError error
}

func (client *fakePullClient) ListPullRequests(_ context.Context, _, _ string, page int) ([]review.PullRequest, bool, error) {
	if client.listError != nil {
		return nil, false, client.listError
	}
	pulls := slices.Clone(client.pages[page])
	_, hasNext := client.pages[page+1]
	return pulls, hasNext, nil
}

func TestRunScansEntireBoundaryPage(t *testing.T) {
	boundary := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{
			1: {
				{Number: 9, State: "closed", AuthorAssociation: "MEMBER", CreatedAt: boundary},
				{Number: 11, State: "open", AuthorAssociation: "MEMBER", CreatedAt: boundary},
				{Number: 8, State: "closed", AuthorAssociation: "MEMBER", CreatedAt: boundary.Add(-time.Second)},
			},
		},
	}
	current := state.New(10)
	current.HighWaterCreatedAt = boundary
	store := &fakeStateStore{exists: true, current: current}
	var reviewed []int
	poller := New(client, store, "microsoft", "vscode", 5, func(_ context.Context, pull review.PullRequest) error {
		reviewed = append(reviewed, pull.Number)
		return nil
	})

	if _, err := poller.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []int{11}) {
		t.Fatalf("reviewed = %v", reviewed)
	}
}

func TestRunSavesCompletedDeferredWorkOnLaterFailure(t *testing.T) {
	expected := errors.New("list failed")
	current := state.New(20)
	current.PendingDrafts = []int{18}
	client := &fakePullClient{
		pulls: map[int]review.PullRequest{
			18: {Number: 18, State: "open", AuthorAssociation: "MEMBER"},
		},
		listError: expected,
	}
	store := &fakeStateStore{exists: true, current: current}
	poller := New(client, store, "microsoft", "vscode", 5, func(context.Context, review.PullRequest) error {
		return nil
	})

	_, err := poller.Run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
	if len(store.current.PendingDrafts) != 0 || len(store.saves) != 1 {
		t.Fatalf("state = %+v, saves = %d", store.current, len(store.saves))
	}
}

func (client *fakePullClient) GetPullRequest(_ context.Context, _, _ string, number int) (review.PullRequest, error) {
	return client.pulls[number], nil
}

type fakeStateStore struct {
	current state.State
	exists  bool
	saves   []state.State
}

func (store *fakeStateStore) Load(context.Context) (state.State, string, bool, error) {
	return store.current, "state-sha", store.exists, nil
}

func (store *fakeStateStore) Save(_ context.Context, current state.State, _ string) error {
	store.current = current
	store.exists = true
	store.saves = append(store.saves, current)
	return nil
}

func TestRunBootstrapsWithoutReviewing(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{
			1: {{Number: 123, State: "open", AuthorAssociation: "MEMBER"}},
		},
	}
	store := &fakeStateStore{}
	reviewCalls := 0
	poller := New(client, store, "microsoft", "vscode", 5, func(context.Context, review.PullRequest) error {
		reviewCalls++
		return nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Bootstrapped != 123 || reviewCalls != 0 {
		t.Fatalf("result = %+v, review calls = %d", result, reviewCalls)
	}
	if store.current.HighWaterMark != 123 {
		t.Fatalf("high water mark = %d", store.current.HighWaterMark)
	}
}

func TestRunReviewsReadyAndDefersDraft(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{
			1: {
				{Number: 13, State: "open", AuthorAssociation: "MEMBER"},
				{Number: 12, State: "open", Draft: true, AuthorAssociation: "MEMBER"},
				{Number: 11, State: "closed", AuthorAssociation: "MEMBER"},
				{Number: 10, State: "open", AuthorAssociation: "MEMBER"},
			},
		},
	}

	store := &fakeStateStore{exists: true, current: state.New(10)}
	var reviewed []int
	poller := New(client, store, "microsoft", "vscode", 5, func(_ context.Context, pull review.PullRequest) error {
		reviewed = append(reviewed, pull.Number)
		return nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []int{13}) {
		t.Fatalf("reviewed = %v", reviewed)
	}
	if !slices.Equal(result.Deferred, []int{12}) || !slices.Equal(result.Skipped, []int{11}) {
		t.Fatalf("result = %+v", result)
	}
	if store.current.HighWaterMark != 13 || !slices.Equal(store.current.PendingDrafts, []int{12}) {
		t.Fatalf("state = %+v", store.current)
	}
}

func TestRunSkipsNonTeamPullRequestsWithoutDeferringDrafts(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{
			1: {
				{Number: 13, State: "open", Draft: true, AuthorLogin: "external", AuthorAssociation: "CONTRIBUTOR"},
				{Number: 12, State: "open", AuthorLogin: "outside-collaborator", AuthorAssociation: "COLLABORATOR"},
				{Number: 11, State: "open", AuthorLogin: "teammate", AuthorAssociation: "MEMBER"},
				{Number: 10, State: "open", AuthorAssociation: "MEMBER"},
			},
		},
	}
	store := &fakeStateStore{exists: true, current: state.New(10)}
	var reviewed []int
	poller := New(client, store, "microsoft", "vscode", 5, func(_ context.Context, pull review.PullRequest) error {
		reviewed = append(reviewed, pull.Number)
		return nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewed, []int{11}) {
		t.Fatalf("reviewed = %v", reviewed)
	}
	if !slices.Equal(result.Skipped, []int{12, 13}) {
		t.Fatalf("skipped = %v", result.Skipped)
	}
	if len(result.Deferred) != 0 || len(store.current.PendingDrafts) != 0 {
		t.Fatalf("deferred = %v, pending drafts = %v", result.Deferred, store.current.PendingDrafts)
	}
	if store.current.HighWaterMark != 13 {
		t.Fatalf("high water mark = %d, want 13", store.current.HighWaterMark)
	}
}

func TestRunRemovesDeferredDraftThatIsNotTeamAuthored(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{1: {{Number: 20, AuthorAssociation: "MEMBER"}}},
		pulls: map[int]review.PullRequest{
			18: {
				Number:            18,
				State:             "open",
				Draft:             true,
				AuthorLogin:       "external",
				AuthorAssociation: "CONTRIBUTOR",
			},
		},
	}
	current := state.New(20)
	current.PendingDrafts = []int{18}
	store := &fakeStateStore{exists: true, current: current}
	reviewCalls := 0
	poller := New(client, store, "microsoft", "vscode", 5, func(context.Context, review.PullRequest) error {
		reviewCalls++
		return nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reviewCalls != 0 || !slices.Equal(result.Skipped, []int{18}) || len(store.current.PendingDrafts) != 0 {
		t.Fatalf("review calls = %d, result = %+v, state = %+v", reviewCalls, result, store.current)
	}
}

func TestRunReviewsDraftWhenReady(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{1: {{Number: 20, AuthorAssociation: "MEMBER"}}},
		pulls: map[int]review.PullRequest{
			18: {Number: 18, State: "open", Draft: false, AuthorAssociation: "MEMBER"},
		},
	}
	current := state.New(20)
	current.PendingDrafts = []int{18}
	store := &fakeStateStore{exists: true, current: current}
	poller := New(client, store, "microsoft", "vscode", 5, func(context.Context, review.PullRequest) error {
		return nil
	})

	result, err := poller.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Reviewed, []int{18}) || len(store.current.PendingDrafts) != 0 {
		t.Fatalf("result = %+v, state = %+v", result, store.current)
	}
}

func TestRunDoesNotAdvanceFailedReview(t *testing.T) {
	client := &fakePullClient{
		pages: map[int][]review.PullRequest{
			1: {
				{Number: 12, State: "open", AuthorAssociation: "MEMBER"},
				{Number: 11, State: "open", AuthorAssociation: "MEMBER"},
				{Number: 10, State: "open", AuthorAssociation: "MEMBER"},
			},
		},
	}
	store := &fakeStateStore{exists: true, current: state.New(10)}
	expected := errors.New("review failed")
	poller := New(client, store, "microsoft", "vscode", 5, func(context.Context, review.PullRequest) error {
		return expected
	})

	_, err := poller.Run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
	if store.current.HighWaterMark != 10 {
		t.Fatalf("high water mark = %d, want 10", store.current.HighWaterMark)
	}
}
