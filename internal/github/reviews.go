package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type reviewResponse struct {
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func ReviewMarker(number int, headSHA string) string {
	return fmt.Sprintf("<!-- rob-reviewer:v1 pr=%d head=%s -->", number, headSHA)
}

func (client *Client) HasReviewMarker(ctx context.Context, owner, repo string, number int, marker string) (bool, error) {
	authenticatedLogin, err := client.authenticatedLogin(ctx)
	if err != nil {
		return false, err
	}
	for page := 1; ; page++ {
		values := url.Values{
			"per_page": {"100"},
			"page":     {strconv.Itoa(page)},
		}
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?%s", url.PathEscape(owner), url.PathEscape(repo), number, values.Encode())
		responseBody, headers, err := client.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return false, err
		}
		var reviews []reviewResponse
		if err := json.Unmarshal(responseBody, &reviews); err != nil {
			return false, fmt.Errorf("decode pull request reviews: %w", err)
		}
		for _, candidate := range reviews {
			if strings.EqualFold(candidate.User.Login, authenticatedLogin) && strings.Contains(candidate.Body, marker) {
				return true, nil
			}
		}
		if !strings.Contains(headers.Get("Link"), `rel="next"`) {
			return false, nil
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
	Event    string          `json:"event"`
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments"`
}

func (client *Client) CreateReview(ctx context.Context, owner, repo string, number int, request CreateReviewRequest) error {
	encodedBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode review request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, _, err := client.do(ctx, http.MethodPost, path, encodedBody); err != nil {
		return fmt.Errorf("create pull request review: %w", err)
	}
	return nil
}

func (client *Client) PublishReview(ctx context.Context, owner, repo string, number int, request review.ReviewRequest) error {
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
		Event:    request.Event,
		Body:     request.Body,
		Comments: comments,
	})
}
