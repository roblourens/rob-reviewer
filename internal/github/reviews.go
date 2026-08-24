package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type reviewResponse struct {
	ID    int64  `json:"id"`
	Body  string `json:"body"`
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
}

func ReviewMarker(number int, headSHA string) string {
	return fmt.Sprintf("<!-- rob-reviewer:v1 pr=%d head=%s -->", number, headSHA)
}

func (client *Client) HasReviewMarker(ctx context.Context, owner, repo string, number int, marker string) (bool, error) {
	existing, err := client.FindReviewMarker(ctx, owner, repo, number, marker)
	return existing != nil, err
}

func (client *Client) FindReviewMarker(
	ctx context.Context,
	owner, repo string,
	number int,
	marker string,
) (*review.ExistingReview, error) {
	authenticatedLogin, err := client.authenticatedLogin(ctx)
	if err != nil {
		return nil, err
	}
	for page := 1; ; page++ {
		values := url.Values{
			"per_page": {"100"},
			"page":     {strconv.Itoa(page)},
		}
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?%s", url.PathEscape(owner), url.PathEscape(repo), number, values.Encode())
		responseBody, headers, err := client.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var reviews []reviewResponse
		if err := json.Unmarshal(responseBody, &reviews); err != nil {
			return nil, fmt.Errorf("decode pull request reviews: %w", err)
		}
		for _, candidate := range reviews {
			if strings.EqualFold(candidate.User.Login, authenticatedLogin) && strings.Contains(candidate.Body, marker) {
				return &review.ExistingReview{ID: candidate.ID, State: candidate.State, Body: candidate.Body}, nil
			}
		}
		if !strings.Contains(headers.Get("Link"), `rel="next"`) {
			return nil, nil
		}
	}
}

type ReviewComment struct {
	Path string      `json:"path"`
	Line int         `json:"line"`
	Side review.Side `json:"side"`
	Body string      `json:"body"`
}

type CreateReviewRequest struct {
	CommitID string          `json:"commit_id"`
	Event    string          `json:"event,omitempty"`
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments"`
}

func (client *Client) CreateReview(ctx context.Context, owner, repo string, number int, request CreateReviewRequest) (int64, error) {
	encodedBody, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("encode review request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repo), number)
	responseBody, _, err := client.do(ctx, http.MethodPost, path, encodedBody)
	if err != nil {
		return 0, fmt.Errorf("create pull request review: %w", err)
	}
	var response reviewResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return 0, fmt.Errorf("decode created pull request review: %w", err)
	}
	if response.ID == 0 {
		return 0, errors.New("created pull request review has no ID")
	}
	return response.ID, nil
}

func (client *Client) CreatePendingReview(
	ctx context.Context,
	owner, repo string,
	number int,
	request review.ReviewRequest,
) (int64, error) {
	comments := make([]ReviewComment, 0, len(request.Comments))
	for _, comment := range request.Comments {
		comments = append(comments, ReviewComment{
			Path: comment.Path,
			Line: comment.Line,
			Side: comment.Side,
			Body: comment.Body,
		})
	}
	return client.CreateReview(ctx, owner, repo, number, CreateReviewRequest{
		CommitID: request.CommitID,
		Body:     request.Body,
		Comments: comments,
	})
}

func (client *Client) SubmitPendingReview(
	ctx context.Context,
	owner, repo string,
	number int,
	reviewID int64,
) error {
	body, err := json.Marshal(struct {
		Event string `json:"event"`
	}{Event: "COMMENT"})
	if err != nil {
		return fmt.Errorf("encode pending review submission: %w", err)
	}
	path := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/reviews/%d/events",
		url.PathEscape(owner),
		url.PathEscape(repo),
		number,
		reviewID,
	)
	if _, _, err := client.do(ctx, http.MethodPost, path, body); err != nil {
		return fmt.Errorf("submit pending pull request review: %w", err)
	}
	return nil
}
