package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListPullRequestsMapsResponseAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/microsoft/vscode/pulls" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Link", `<https://api.github.com/resource?page=2>; rel="next"`)
		fmt.Fprint(writer, `[{"number":7,"title":"Improve","body":"Body","state":"open","draft":false,"html_url":"https://example.test/7","base":{"ref":"main","sha":"base"},"head":{"ref":"feature","sha":"head"},"created_at":"2026-08-01T00:00:00Z"}]`)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.Client(), "token", server.URL)
	pulls, hasNext, err := client.ListPullRequests(context.Background(), "microsoft", "vscode", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !hasNext || len(pulls) != 1 || pulls[0].Number != 7 || pulls[0].HeadSHA != "head" {
		t.Fatalf("pulls = %+v, hasNext = %v", pulls, hasNext)
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
