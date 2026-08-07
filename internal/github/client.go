package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roblourens/rob-reviewer/internal/review"
)

const defaultBaseURL = "https://api.github.com"

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string

	userMutex sync.Mutex
	userLogin string
}

func (client *Client) authenticatedLogin(ctx context.Context) (string, error) {
	client.userMutex.Lock()
	defer client.userMutex.Unlock()
	if client.userLogin != "" {
		return client.userLogin, nil
	}
	responseBody, _, err := client.do(ctx, http.MethodGet, "/user", nil)
	if err != nil {
		return "", fmt.Errorf("get authenticated GitHub user: %w", err)
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(responseBody, &user); err != nil {
		return "", fmt.Errorf("decode authenticated GitHub user: %w", err)
	}
	if strings.TrimSpace(user.Login) == "" {
		return "", errors.New("authenticated GitHub user response has no login")
	}
	client.userLogin = user.Login
	return client.userLogin, nil
}

func NewClient(httpClient *http.Client, token string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    defaultBaseURL,
		token:      strings.TrimSpace(token),
	}
}

func NewClientWithBaseURL(httpClient *http.Client, token, baseURL string) *Client {
	client := NewClient(httpClient, token)
	client.baseURL = strings.TrimRight(baseURL, "/")
	return client
}

type pullResponse struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	CreatedAt time.Time `json:"created_at"`
	Base      struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
}

func (client *Client) ListPullRequests(ctx context.Context, owner, repo string, page int) ([]review.PullRequest, bool, error) {
	values := url.Values{
		"state":     {"all"},
		"sort":      {"created"},
		"direction": {"desc"},
		"per_page":  {"100"},
		"page":      {strconv.Itoa(page)},
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls?%s", url.PathEscape(owner), url.PathEscape(repo), values.Encode())
	var response []pullResponse
	responseBody, headers, err := client.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, false, fmt.Errorf("decode pull request list: %w", err)
	}
	pulls := make([]review.PullRequest, 0, len(response))
	for _, pull := range response {
		pulls = append(pulls, pull.toReviewPullRequest())
	}
	return pulls, strings.Contains(headers.Get("Link"), `rel="next"`), nil
}

func (client *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (review.PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	var response pullResponse
	responseBody, _, err := client.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return review.PullRequest{}, err
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return review.PullRequest{}, fmt.Errorf("decode pull request: %w", err)
	}
	return response.toReviewPullRequest(), nil
}

func (pull pullResponse) toReviewPullRequest() review.PullRequest {
	return review.PullRequest{
		Number:    pull.Number,
		Title:     pull.Title,
		Body:      pull.Body,
		URL:       pull.HTMLURL,
		State:     pull.State,
		Draft:     pull.Draft,
		BaseRef:   pull.Base.Ref,
		BaseSHA:   pull.Base.SHA,
		HeadRef:   pull.Head.Ref,
		HeadSHA:   pull.Head.SHA,
		CreatedAt: pull.CreatedAt,
	}
}

type Repository struct {
	DefaultBranch string `json:"default_branch"`
}

func (client *Client) GetRepository(ctx context.Context, owner, repo string) (Repository, error) {
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	var repository Repository
	responseBody, _, err := client.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Repository{}, err
	}
	if err := json.Unmarshal(responseBody, &repository); err != nil {
		return Repository{}, fmt.Errorf("decode repository: %w", err)
	}
	return repository, nil
}

type gitRefResponse struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

func (client *Client) EnsureBranch(ctx context.Context, owner, repo, branch string) error {
	repository, err := client.GetRepository(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("get repository for state branch: %w", err)
	}

	refPath := fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", url.PathEscape(owner), url.PathEscape(repo), escapePath(repository.DefaultBranch))
	var ref gitRefResponse
	responseBody, _, err := client.do(ctx, http.MethodGet, refPath, nil)
	if err != nil {
		return fmt.Errorf("get default branch ref: %w", err)
	}
	if err := json.Unmarshal(responseBody, &ref); err != nil {
		return fmt.Errorf("decode default branch ref: %w", err)
	}

	body := struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}{
		Ref: "refs/heads/" + branch,
		SHA: ref.Object.SHA,
	}
	createPath := fmt.Sprintf("/repos/%s/%s/git/refs", url.PathEscape(owner), url.PathEscape(repo))
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode state branch request: %w", err)
	}
	if _, _, err := client.do(ctx, http.MethodPost, createPath, encodedBody); err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) &&
			apiError.StatusCode == http.StatusUnprocessableEntity &&
			strings.Contains(strings.ToLower(apiError.Message), "reference already exists") {
			return nil
		}
		return fmt.Errorf("create state branch from %s: %w", repository.DefaultBranch, err)
	}
	return nil
}

type FileContent struct {
	Content []byte
	SHA     string
}

type contentResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	SHA      string `json:"sha"`
}

func (client *Client) GetContent(ctx context.Context, owner, repo, path, branch string) (FileContent, bool, error) {
	values := url.Values{"ref": {branch}}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s?%s", url.PathEscape(owner), url.PathEscape(repo), escapePath(path), values.Encode())
	var response contentResponse
	responseBody, _, err := client.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
			return FileContent{}, false, nil
		}
		return FileContent{}, false, err
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return FileContent{}, false, fmt.Errorf("decode content response: %w", err)
	}
	if response.Encoding != "base64" {
		return FileContent{}, false, fmt.Errorf("unsupported content encoding %q", response.Encoding)
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(response.Content, "\n", ""))
	if err != nil {
		return FileContent{}, false, fmt.Errorf("decode content: %w", err)
	}
	return FileContent{Content: content, SHA: response.SHA}, true, nil
}

func (client *Client) PutContent(ctx context.Context, owner, repo, path, branch, message string, content []byte, sha string) error {
	body := struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha,omitempty"`
	}{
		Message: message,
		Content: base64.StdEncoding.EncodeToString(content),
		Branch:  branch,
		SHA:     sha,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), escapePath(path))
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode content request: %w", err)
	}
	if _, _, err := client.do(ctx, http.MethodPut, apiPath, encodedBody); err != nil {
		return err
	}
	return nil
}

type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Message    string
}

func (err *APIError) Error() string {
	return fmt.Sprintf("GitHub API %s %s returned %d: %s", err.Method, err.Path, err.StatusCode, err.Message)
}

func (client *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, http.Header, error) {
	var requestBody io.Reader
	if body != nil {
		requestBody = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, requestBody)
	if err != nil {
		return nil, nil, fmt.Errorf("create GitHub request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "rob-reviewer")
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("call GitHub API: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read GitHub response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		var errorBody struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(responseBody, &errorBody) == nil && errorBody.Message != "" {
			message = errorBody.Message
		}
		return nil, response.Header, &APIError{
			StatusCode: response.StatusCode,
			Method:     method,
			Path:       path,
			Message:    message,
		}
	}
	return responseBody, response.Header, nil
}

func escapePath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
