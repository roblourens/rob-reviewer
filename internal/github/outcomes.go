package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/roblourens/rob-reviewer/internal/outcomes"
	"github.com/roblourens/rob-reviewer/internal/review"
)

var _ outcomes.Reader = (*Client)(nil)

// Large or changing conversations must fail rather than become partial snapshots.
const (
	outcomePageSize    = 100
	maxOutcomeThreads  = 10000
	maxOutcomeComments = 100000
	maxOutcomeRequests = 2000
	maxOutcomeBytes    = 64 << 20
)

const outcomeCommentsSelection = `
	totalCount
	pageInfo { hasNextPage endCursor }
	nodes {
		databaseId url body path createdAt updatedAt
		author { login __typename }
		replyTo { databaseId }
		pullRequestReview { body state }
	}`

const outcomeThreadsQuery = `
query CommentOutcomes($owner: String!, $repo: String!, $number: Int!, $after: String) {
	repository(owner: $owner, name: $repo) {
		pullRequest(number: $number) {
			url state author { login __typename }
			reviewThreads(first: 100, after: $after) {
				totalCount
				pageInfo { hasNextPage endCursor }
				nodes {
					id isResolved isOutdated resolvedBy { login __typename }
					comments(first: 100) {` + outcomeCommentsSelection + `}
				}
			}
		}
	}
}`

const outcomeRepliesQuery = `
query CommentOutcomeReplies($threadID: ID!, $after: String!) {
	node(id: $threadID) {
		... on PullRequestReviewThread {
			id
			comments(first: 100, after: $after) {` + outcomeCommentsSelection + `}
		}
	}
}`

type outcomeQueryRequest struct {
	Query     string                `json:"query"`
	Variables outcomeQueryVariables `json:"variables"`
}

type outcomeQueryVariables struct {
	Owner    string  `json:"owner,omitempty"`
	Repo     string  `json:"repo,omitempty"`
	Number   int     `json:"number,omitempty"`
	ThreadID string  `json:"threadID,omitempty"`
	After    *string `json:"after,omitempty"`
}

type outcomeQueryResponse struct {
	Data *struct {
		Repository *struct {
			PullRequest *outcomePullRequest `json:"pullRequest"`
		} `json:"repository"`
		Node *outcomeThread `json:"node"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type outcomePullRequest struct {
	URL           string                            `json:"url"`
	State         string                            `json:"state"`
	Author        *outcomeActor                     `json:"author"`
	ReviewThreads *outcomeConnection[outcomeThread] `json:"reviewThreads"`
}

type outcomeActor struct {
	Login    string `json:"login"`
	TypeName string `json:"__typename"`
}

func (actor *outcomeActor) valid() bool {
	return actor != nil && strings.TrimSpace(actor.Login) != "" && strings.TrimSpace(actor.TypeName) != ""
}

// Nullable GraphQL fields must distinguish a legitimate null from an omitted field.
type outcomeNullable[T any] struct {
	Value   *T
	Present bool
}

func (value *outcomeNullable[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	return json.Unmarshal(data, &value.Value)
}

type outcomeThread struct {
	ID         string                             `json:"id"`
	Resolved   *bool                              `json:"isResolved"`
	Outdated   *bool                              `json:"isOutdated"`
	ResolvedBy outcomeNullable[outcomeActor]      `json:"resolvedBy"`
	Comments   *outcomeConnection[outcomeComment] `json:"comments"`
}

type outcomeComment struct {
	ID        *int64        `json:"databaseId"`
	URL       string        `json:"url"`
	Author    *outcomeActor `json:"author"`
	Body      *string       `json:"body"`
	Path      string        `json:"path"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	ReplyTo   outcomeNullable[struct {
		ID *int64 `json:"databaseId"`
	}] `json:"replyTo"`
	Review *struct {
		Body  *string `json:"body"`
		State string  `json:"state"`
	} `json:"pullRequestReview"`
}

type outcomeConnection[T any] struct {
	TotalCount *int             `json:"totalCount"`
	PageInfo   *outcomePageInfo `json:"pageInfo"`
	Nodes      []*T             `json:"nodes"`
}

type outcomePageInfo struct {
	HasNextPage *bool   `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

type outcomePagination struct {
	total    *int
	received int
	cursors  map[string]bool
}

func (pagination *outcomePagination) advance(total *int, size int, info *outcomePageInfo, limit int) (*string, error) {
	if total == nil || *total < 0 || info == nil || info.HasNextPage == nil {
		return nil, errors.New("incomplete pagination data")
	}
	if *total > limit || size > outcomePageSize {
		return nil, errors.New("comment outcomes pagination limit exceeded")
	}
	if pagination.total == nil {
		pagination.total = total
		pagination.cursors = make(map[string]bool)
	} else if *pagination.total != *total {
		return nil, errors.New("pagination total changed during retrieval")
	}
	pagination.received += size
	if pagination.received > *total || (size == 0 && (*total != 0 || *info.HasNextPage)) {
		return nil, errors.New("inconsistent pagination count")
	}
	if size > 0 {
		if info.EndCursor == nil || strings.TrimSpace(*info.EndCursor) == "" {
			return nil, errors.New("missing pagination cursor")
		}
		if pagination.cursors[*info.EndCursor] {
			return nil, errors.New("repeated pagination cursor")
		}
		pagination.cursors[*info.EndCursor] = true
	}
	if *info.HasNextPage {
		if pagination.received >= *total {
			return nil, errors.New("pagination continues beyond total count")
		}
		return info.EndCursor, nil
	}
	if pagination.received != *total {
		return nil, errors.New("incomplete pagination: missing nodes")
	}
	return nil, nil
}

type outcomeRead struct {
	client     *Client
	requests   int
	bytes      int
	commentIDs map[int64]bool
}

func (read *outcomeRead) query(ctx context.Context, request outcomeQueryRequest) (*outcomeQueryResponse, error) {
	if read.requests >= maxOutcomeRequests {
		return nil, errors.New("comment outcomes request limit exceeded")
	}
	read.requests++
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode comment outcomes query: %w", err)
	}
	body, _, err = read.client.do(ctx, http.MethodPost, "/graphql", body)
	if err != nil {
		return nil, err
	}
	read.bytes += len(body)
	// client.do caps individual responses at 10 MiB.
	if len(body) >= 10<<20 || read.bytes > maxOutcomeBytes {
		return nil, errors.New("comment outcomes response byte limit exceeded")
	}
	var response outcomeQueryResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode comment outcomes: %w", err)
	}
	if len(response.Errors) != 0 {
		messages := make([]string, 0, len(response.Errors))
		for _, graphError := range response.Errors {
			messages = append(messages, graphError.Message)
		}
		return nil, fmt.Errorf("GitHub GraphQL errors: %s", strings.Join(messages, "; "))
	}
	if response.Data == nil {
		return nil, errors.New("comment outcomes data is missing or inaccessible")
	}
	return &response, nil
}

// GetCommentOutcomes returns only complete conversations rooted in this client's
// published, marked reviews. It never returns a partial snapshot on failure.
func (client *Client) GetCommentOutcomes(ctx context.Context, owner, repo string, number int) (outcomes.Snapshot, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || number <= 0 {
		return outcomes.Snapshot{}, errors.New("comment outcomes require an owner, repository, and positive PR number")
	}
	login, err := client.authenticatedLogin(ctx)
	if err != nil {
		return outcomes.Snapshot{}, err
	}
	read := outcomeRead{client: client, commentIDs: make(map[int64]bool)}
	snapshot, err := read.snapshot(ctx, owner, repo, number, login)
	if err != nil {
		return outcomes.Snapshot{}, fmt.Errorf("get comment outcomes for %s/%s#%d: %w", owner, repo, number, err)
	}
	return snapshot, nil
}

func (read *outcomeRead) snapshot(ctx context.Context, owner, repo string, number int, login string) (outcomes.Snapshot, error) {
	snapshot := outcomes.Snapshot{Number: number, Threads: []outcomes.Thread{}}
	var pagination outcomePagination
	var after *string
	threadIDs := make(map[string]bool)
	for {
		response, err := read.query(ctx, outcomeQueryRequest{
			Query:     outcomeThreadsQuery,
			Variables: outcomeQueryVariables{Owner: owner, Repo: repo, Number: number, After: after},
		})
		if err != nil {
			return snapshot, err
		}
		if response.Data.Repository == nil || response.Data.Repository.PullRequest == nil {
			return snapshot, errors.New("pull request is missing or inaccessible")
		}
		pull := response.Data.Repository.PullRequest
		if strings.TrimSpace(pull.URL) == "" || !pull.Author.valid() {
			return snapshot, errors.New("incomplete pull request metadata")
		}
		switch pull.State {
		case "OPEN", "CLOSED", "MERGED":
		default:
			return snapshot, fmt.Errorf("invalid pull request state %q", pull.State)
		}
		if snapshot.URL != "" && (snapshot.URL != pull.URL || snapshot.Author != pull.Author.Login || snapshot.State != pull.State) {
			return snapshot, errors.New("pull request metadata changed during retrieval")
		}
		snapshot.URL, snapshot.Author, snapshot.State = pull.URL, pull.Author.Login, pull.State
		connection := pull.ReviewThreads
		if connection == nil || connection.Nodes == nil {
			return snapshot, errors.New("review threads are missing or inaccessible")
		}
		after, err = pagination.advance(connection.TotalCount, len(connection.Nodes), connection.PageInfo, maxOutcomeThreads)
		if err != nil {
			return snapshot, fmt.Errorf("review threads: %w", err)
		}
		for _, source := range connection.Nodes {
			if source == nil || strings.TrimSpace(source.ID) == "" || source.Resolved == nil || source.Outdated == nil ||
				!source.ResolvedBy.Present || (source.ResolvedBy.Value != nil && !source.ResolvedBy.Value.valid()) {
				return snapshot, errors.New("incomplete review thread")
			}
			if threadIDs[source.ID] {
				return snapshot, fmt.Errorf("duplicate review thread %q", source.ID)
			}
			threadIDs[source.ID] = true
			thread, selected, err := read.thread(ctx, source, login, review.ReviewMarkerPrefix(number))
			if err != nil {
				return snapshot, fmt.Errorf("review thread %q: %w", source.ID, err)
			}
			if selected {
				snapshot.Threads = append(snapshot.Threads, thread)
			}
		}
		if after == nil {
			return snapshot, nil
		}
	}
}

func (read *outcomeRead) thread(ctx context.Context, source *outcomeThread, login, marker string) (outcomes.Thread, bool, error) {
	thread := outcomes.Thread{ID: source.ID, Resolved: *source.Resolved, Outdated: *source.Outdated}
	if source.ResolvedBy.Value != nil {
		thread.ResolvedBy = source.ResolvedBy.Value.Login
	}
	connection := source.Comments
	var pagination outcomePagination
	var root *outcomeComment
	var previous time.Time
	selected := false
	for {
		if connection == nil || connection.Nodes == nil || len(connection.Nodes) == 0 {
			return thread, false, errors.New("thread comments are missing or inaccessible")
		}
		after, err := pagination.advance(connection.TotalCount, len(connection.Nodes), connection.PageInfo, maxOutcomeComments)
		if err != nil {
			return thread, false, err
		}
		for _, comment := range connection.Nodes {
			if err := read.validateComment(comment); err != nil {
				return thread, false, err
			}
			if comment.CreatedAt.Before(previous) {
				return thread, false, errors.New("comments are not in chronological order")
			}
			if root == nil {
				root = comment
				selected = comment.ReplyTo.Value == nil && strings.EqualFold(comment.Author.Login, login) &&
					comment.Review.State != "PENDING" && strings.Contains(*comment.Review.Body, marker)
			} else if root.ReplyTo.Value == nil &&
				(comment.ReplyTo.Value == nil || *comment.ReplyTo.Value.ID != *root.ID) {
				return thread, false, errors.New("review comment does not reply to the thread root")
			}
			previous = comment.CreatedAt
			if selected {
				thread.Comments = append(thread.Comments, outcomes.Comment{
					ID: *comment.ID, URL: comment.URL, Author: comment.Author.Login,
					Bot: comment.Author.TypeName == "Bot", Body: *comment.Body, Path: comment.Path,
					CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt,
				})
			}
		}
		if after == nil {
			return thread, selected, nil
		}
		response, err := read.query(ctx, outcomeQueryRequest{
			Query:     outcomeRepliesQuery,
			Variables: outcomeQueryVariables{ThreadID: source.ID, After: after},
		})
		if err != nil {
			return thread, false, err
		}
		if response.Data.Node == nil || response.Data.Node.ID != source.ID {
			return thread, false, errors.New("paginated review thread is missing, inaccessible, or mismatched")
		}
		connection = response.Data.Node.Comments
	}
}

func (read *outcomeRead) validateComment(comment *outcomeComment) error {
	if comment == nil || comment.ID == nil || *comment.ID <= 0 || strings.TrimSpace(comment.URL) == "" || strings.TrimSpace(comment.Path) == "" ||
		!comment.Author.valid() || comment.Body == nil || comment.CreatedAt.IsZero() || comment.UpdatedAt.IsZero() ||
		comment.UpdatedAt.Before(comment.CreatedAt) || !comment.ReplyTo.Present ||
		comment.Review == nil || comment.Review.Body == nil {
		return errors.New("incomplete review comment")
	}
	if parent := comment.ReplyTo.Value; parent != nil && (parent.ID == nil || *parent.ID <= 0 || *parent.ID == *comment.ID) {
		return errors.New("invalid review comment reply parent")
	}
	switch comment.Review.State {
	case "PENDING", "COMMENTED", "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
	default:
		return fmt.Errorf("invalid review state %q", comment.Review.State)
	}
	if read.commentIDs[*comment.ID] {
		return fmt.Errorf("duplicate review comment %d", *comment.ID)
	}
	if len(read.commentIDs) >= maxOutcomeComments {
		return errors.New("comment outcomes total comment limit exceeded")
	}
	read.commentIDs[*comment.ID] = true
	return nil
}
