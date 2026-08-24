package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

func TestListPullRequestsMapsResponseAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/microsoft/vscode/pulls" {
			t.Fatalf("path = %q", request.URL.Path)
		}

		writer.Header().Set("Link", `<https://api.github.com/resource?page=2>; rel="next"`)
		if request.URL.Query().Get("state") != "all" || request.URL.Query().Get("sort") != "created" {
			t.Fatalf("query = %q", request.URL.RawQuery)
		}
		fmt.Fprint(writer, `[{"number":7,"title":"Improve","body":"Body","state":"open","draft":false,"html_url":"https://example.test/7","author_association":"MEMBER","user":{"login":"teammate"},"base":{"ref":"main","sha":"base"},"head":{"ref":"feature","sha":"head"},"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-02T00:00:00Z","labels":[{"name":"performance-reviewer:skip"}]}]`)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.Client(), "token", server.URL)
	pulls, hasNext, err := client.ListPullRequests(context.Background(), "microsoft", "vscode", 1)
	if err != nil {
		t.Fatal(err)
	}

	if !hasNext || len(pulls) != 1 || pulls[0].Number != 7 || pulls[0].HeadSHA != "head" ||
		pulls[0].AuthorLogin != "teammate" || pulls[0].AuthorAssociation != "MEMBER" ||
		!pulls[0].HasAnyLabel([]string{"PERFORMANCE-REVIEWER:SKIP"}) {
		t.Fatalf("pulls = %+v, hasNext = %v", pulls, hasNext)
	}
}

func TestListUpdatedAndOpenPullRequestsUseExpectedQueries(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		queries = append(queries, request.URL.Query().Encode())
		fmt.Fprint(writer, `[]`)
	}))
	defer server.Close()
	client := NewClientWithBaseURL(server.Client(), "token", server.URL)

	if _, _, err := client.ListOpenPullRequests(context.Background(), "microsoft", "vscode", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ListUpdatedPullRequests(context.Background(), "microsoft", "vscode", 2); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 ||
		!strings.Contains(queries[0], "state=open") || !strings.Contains(queries[0], "sort=created") ||
		!strings.Contains(queries[1], "state=all") || !strings.Contains(queries[1], "sort=updated") ||
		!strings.Contains(queries[1], "page=2") {
		t.Fatalf("queries = %v", queries)
	}
}

func TestCreateAndSubmitPendingReview(t *testing.T) {
	var created CreateReviewRequest
	var submitted struct {
		Event string `json:"event"`
	}
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/repos/microsoft/vscode/pulls/7/reviews":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			writer.WriteHeader(http.StatusCreated)
			fmt.Fprint(writer, `{"id":99}`)
		case request.Method == http.MethodPost && request.URL.Path == "/repos/microsoft/vscode/pulls/7/reviews/99/events":
			if err := json.NewDecoder(request.Body).Decode(&submitted); err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(writer, `{}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/repos/microsoft/vscode/pulls/7/reviews/99":
			deleted = true
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.Client(), "token", server.URL)
	reviewID, err := client.CreatePendingReview(context.Background(), "microsoft", "vscode", 7, review.ReviewRequest{
		CommitID: "head",
		Body:     "body",
		Comments: []review.Comment{{Path: "src/file.ts", Line: 3, Side: review.SideRight, Body: "comment"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewID != 99 || created.Event != "" || created.CommitID != "head" || len(created.Comments) != 1 {
		t.Fatalf("review ID=%d request=%+v", reviewID, created)
	}
	if err := client.SubmitPendingReview(context.Background(), "microsoft", "vscode", 7, reviewID); err != nil {
		t.Fatal(err)
	}
	if submitted.Event != "COMMENT" {
		t.Fatalf("submitted event = %q", submitted.Event)
	}
	if err := client.DeletePendingReview(context.Background(), "microsoft", "vscode", 7, reviewID); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("pending review was not deleted")
	}
}

func TestEnsureBranchUsesDefaultBranchRef(t *testing.T) {
	var requestedRef string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/owner/repo":
			fmt.Fprint(writer, `{"default_branch":"trunk"}`)
		case request.Method == http.MethodGet:
			requestedRef = request.URL.Path
			fmt.Fprint(writer, `{"object":{"sha":"abc"}}`)
		case request.Method == http.MethodPost:
			writer.WriteHeader(http.StatusCreated)
			fmt.Fprint(writer, `{}`)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.Client(), "token", server.URL)
	if err := client.EnsureBranch(context.Background(), "owner", "repo", "reviewer-state"); err != nil {
		t.Fatal(err)
	}
	if requestedRef != "/repos/owner/repo/git/ref/heads/trunk" {
		t.Fatalf("requested ref = %q", requestedRef)
	}
}

func TestHasReviewMarkerRequiresAuthenticatedAuthor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/user":
			fmt.Fprint(writer, `{"login":"review-bot"}`)
		case "/repos/microsoft/vscode/pulls/7/reviews":
			fmt.Fprint(writer, `[
				{"body":"attacker-marker","user":{"login":"attacker"}},
				{"body":"reviewer-marker","user":{"login":"review-bot"}}
			]`)
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.Client(), "token", server.URL)
	exists, err := client.HasReviewMarker(
		context.Background(),
		"microsoft",
		"vscode",
		7,
		"attacker-marker",
	)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("accepted marker from another user")
	}
	exists, err = client.HasReviewMarker(context.Background(), "microsoft", "vscode", 7, "reviewer-marker")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("did not accept marker from authenticated reviewer")
	}
}
