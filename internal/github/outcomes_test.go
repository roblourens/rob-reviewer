package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/roblourens/rob-reviewer/internal/outcomes"
	"github.com/roblourens/rob-reviewer/internal/review"
)

func outcomeTestComment(id int64, author, kind, reviewBody, state string, parent int64) string {
	replyTo := "null"
	if parent != 0 {
		replyTo = fmt.Sprintf(`{"databaseId":%d}`, parent)
	}
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second)
	return fmt.Sprintf(`{
		"databaseId":%d,"url":"https://example.test/comments/%d",
		"author":{"login":%s,"__typename":%s},"body":"Comment %d","path":"src/file.go",
		"createdAt":%s,"updatedAt":%s,"replyTo":%s,
		"pullRequestReview":{"body":%s,"state":%s}
	}`, id, id, strconv.Quote(author), strconv.Quote(kind), id,
		strconv.Quote(created.Format(time.RFC3339)), strconv.Quote(created.Add(time.Minute).Format(time.RFC3339)),
		replyTo, strconv.Quote(reviewBody), strconv.Quote(state))
}

func outcomeTestConnection(nodes []string, total int, next bool, cursor string) string {
	endCursor := "null"
	if cursor != "" {
		endCursor = strconv.Quote(cursor)
	}
	return fmt.Sprintf(`{"totalCount":%d,"pageInfo":{"hasNextPage":%t,"endCursor":%s},"nodes":[%s]}`,
		total, next, endCursor, strings.Join(nodes, ","))
}

func outcomeTestThread(id, comments string) string {
	return fmt.Sprintf(`{"id":%s,"isResolved":false,"isOutdated":false,"resolvedBy":null,"comments":%s}`,
		strconv.Quote(id), comments)
}

func outcomeTestResponse(threads, state string) string {
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{
		"url":"https://example.test/pull/7","state":%s,
		"author":{"login":"pr-author","__typename":"User"},"reviewThreads":%s
	}}}}`, strconv.Quote(state), threads)
}

func outcomeTestNodeResponse(id, comments string) string {
	return fmt.Sprintf(`{"data":{"node":{"id":%s,"comments":%s}}}`, strconv.Quote(id), comments)
}

func outcomeTestClient(t *testing.T, login string, handle func(outcomeQueryRequest) (int, string)) (*Client, *int) {
	t.Helper()
	userCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Error("request did not use the client's credentials")
		}
		if request.Method == http.MethodGet && request.URL.Path == "/user" {
			userCalls++
			fmt.Fprintf(writer, `{"login":%s}`, strconv.Quote(login))
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/graphql" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusBadRequest)
			return
		}
		var query outcomeQueryRequest
		if err := json.NewDecoder(request.Body).Decode(&query); err != nil {
			t.Errorf("decode query: %v", err)
			http.Error(writer, "invalid query", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(strings.TrimSpace(query.Query), "query ") || strings.Contains(query.Query, "mutation") {
			t.Error("expected only read-only GraphQL queries")
		}
		if query.Variables.ThreadID == "" {
			if query.Variables.Owner != "owner" || query.Variables.Repo != "repo" || query.Variables.Number != 7 {
				t.Errorf("PR query variables = %+v", query.Variables)
			}
			for _, field := range []string{"reviewThreads(first: 100, after: $after)", "isResolved", "isOutdated", "resolvedBy"} {
				if !strings.Contains(query.Query, field) {
					t.Errorf("PR query missing %s", field)
				}
			}
		} else if !strings.Contains(query.Query, "comments(first: 100, after: $after)") {
			t.Error("nested comments query lacks pagination")
		}
		for _, field := range []string{"totalCount", "hasNextPage", "endCursor", "databaseId", "url", "body",
			"path", "createdAt", "updatedAt", "__typename", "replyTo", "pullRequestReview"} {
			if !strings.Contains(query.Query, field) {
				t.Errorf("query missing %s", field)
			}
		}
		status, body := handle(query)
		writer.WriteHeader(status)
		fmt.Fprint(writer, body)
	}))
	t.Cleanup(server.Close)
	return NewClientWithBaseURL(server.Client(), "token", server.URL), &userCalls
}

func TestGetCommentOutcomesSelectsAuthenticatedRoots(t *testing.T) {
	marker := review.ReviewMarker(7, "head")
	root := outcomeTestComment(1, "ReViEwEr", "User", marker, "COMMENTED", 0)
	human := outcomeTestComment(2, "pr-author", "User", "Thanks!", "COMMENTED", 1)
	reviewer := outcomeTestComment(3, "reviewer", "User", "", "COMMENTED", 1)
	bot := outcomeTestComment(4, "automation", "Bot", "", "COMMENTED", 1)
	// A bot-looking login alone must not classify a human account as a bot.
	otherHuman := outcomeTestComment(5, "person[bot]", "User", "", "COMMENTED", 1)
	selected := outcomeTestThread("selected", outcomeTestConnection([]string{root, human, reviewer, bot, otherHuman}, 5, false, "comments"))
	selected = strings.Replace(selected, `"isResolved":false`, `"isResolved":true`, 1)
	selected = strings.Replace(selected, `"isOutdated":false`, `"isOutdated":true`, 1)
	selected = strings.Replace(selected, `"resolvedBy":null`, `"resolvedBy":{"login":"resolver","__typename":"User"}`, 1)

	candidates := []string{
		// Marker copied by another human/bot is not a review by the authenticated client.
		outcomeTestComment(10, "other-human", "User", marker, "COMMENTED", 0),
		outcomeTestComment(11, "other-bot", "Bot", marker, "COMMENTED", 0),
		outcomeTestComment(12, "reviewer", "User", "Manual review", "COMMENTED", 0),
		outcomeTestComment(13, "reviewer", "User", review.ReviewMarker(70, "head"), "COMMENTED", 0),
		outcomeTestComment(14, "reviewer", "User", marker, "PENDING", 0),
		outcomeTestComment(15, "reviewer", "User", marker, "COMMENTED", 999),
	}
	// The marker must occur in the enclosing review body, not the inline comment.
	candidates[2] = strings.Replace(candidates[2], `"body":"Comment 12"`, `"body":`+strconv.Quote(marker), 1)
	threads := []string{selected}
	for index, candidate := range candidates {
		threads = append(threads, outcomeTestThread(fmt.Sprintf("ignored-%d", index),
			outcomeTestConnection([]string{candidate}, 1, false, "comment")))
	}
	threads = append(threads, outcomeTestThread("human-root", outcomeTestConnection([]string{
		outcomeTestComment(20, "human", "User", "", "COMMENTED", 0),
		outcomeTestComment(21, "reviewer", "Bot", marker, "COMMENTED", 20),
	}, 2, false, "comment")))
	// Dismissal does not unpublish an existing review or its conversation.
	threads = append(threads, outcomeTestThread("dismissed", outcomeTestConnection([]string{
		outcomeTestComment(30, "reviewer", "Bot", marker, "DISMISSED", 0),
	}, 1, false, "comment")))

	client, userCalls := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
		return http.StatusOK, outcomeTestResponse(outcomeTestConnection(threads, len(threads), false, "threads"), "MERGED")
	})
	for range 2 {
		snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Number != 7 || snapshot.URL != "https://example.test/pull/7" ||
			snapshot.Author != "pr-author" || snapshot.State != "MERGED" || len(snapshot.Threads) != 2 {
			t.Fatalf("snapshot = %+v", snapshot)
		}
		thread := snapshot.Threads[0]
		if thread.ID != "selected" || !thread.Resolved || !thread.Outdated || thread.ResolvedBy != "resolver" {
			t.Fatalf("thread = %+v", thread)
		}
		var expected []outcomes.Comment
		for index, author := range []string{"ReViEwEr", "pr-author", "reviewer", "automation", "person[bot]"} {
			id := int64(index + 1)
			created := time.Date(2026, 9, 1, 0, 0, index+1, 0, time.UTC)
			expected = append(expected, outcomes.Comment{
				ID: id, URL: fmt.Sprintf("https://example.test/comments/%d", id), Author: author, Bot: index == 3,
				Body: fmt.Sprintf("Comment %d", id), Path: "src/file.go", CreatedAt: created, UpdatedAt: created.Add(time.Minute),
			})
		}
		if !reflect.DeepEqual(thread.Comments, expected) {
			t.Fatalf("comments = %+v; expected %+v", thread.Comments, expected)
		}
		if snapshot.Threads[1].ID != "dismissed" || !snapshot.Threads[1].Comments[0].Bot {
			t.Fatalf("dismissed bot thread = %+v", snapshot.Threads[1])
		}
	}
	if *userCalls != 1 {
		t.Fatalf("authenticated user lookups = %d, want cached identity", *userCalls)
	}
}

func TestGetCommentOutcomesPullRequestStates(t *testing.T) {
	for _, state := range []string{"OPEN", "CLOSED", "MERGED"} {
		t.Run(state, func(t *testing.T) {
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				return http.StatusOK, outcomeTestResponse(outcomeTestConnection(nil, 0, false, ""), state)
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err != nil || snapshot.State != state || len(snapshot.Threads) != 0 {
				t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
			}
		})
	}
}

func TestGetCommentOutcomesPaginatesThreadsAndComments(t *testing.T) {
	marker := review.ReviewMarker(7, "head")
	var comments []string
	for index := range 201 {
		parent, author := int64(1), "pr-author"
		if index == 0 {
			parent, author = 0, "reviewer"
		} else if index%2 == 0 {
			author = "reviewer"
		}
		comments = append(comments, outcomeTestComment(int64(index+1), author, "User", marker, "COMMENTED", parent))
	}
	var threads []string
	threads = append(threads, outcomeTestThread("T-0", outcomeTestConnection(comments[:100], 201, true, "comments-100")))
	for index := 1; index < 101; index++ {
		threads = append(threads, outcomeTestThread(fmt.Sprintf("T-%d", index), outcomeTestConnection([]string{
			outcomeTestComment(int64(1000+index), "reviewer", "User", marker, "COMMENTED", 0),
		}, 1, false, "comment")))
	}
	var requests []string
	client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
		after := ""
		if query.Variables.After != nil {
			after = *query.Variables.After
		}
		requests = append(requests, query.Variables.ThreadID+":"+after)
		switch {
		case query.Variables.ThreadID == "" && after == "":
			return http.StatusOK, outcomeTestResponse(outcomeTestConnection(threads[:100], 101, true, "threads-100"), "OPEN")
		case query.Variables.ThreadID == "" && after == "threads-100":
			return http.StatusOK, outcomeTestResponse(outcomeTestConnection(threads[100:], 101, false, "threads-101"), "OPEN")
		case query.Variables.ThreadID == "T-0" && after == "comments-100":
			return http.StatusOK, outcomeTestNodeResponse("T-0", outcomeTestConnection(comments[100:200], 201, true, "comments-200"))
		case query.Variables.ThreadID == "T-0" && after == "comments-200":
			return http.StatusOK, outcomeTestNodeResponse("T-0", outcomeTestConnection(comments[200:], 201, false, "comments-201"))
		default:
			t.Errorf("unexpected pagination variables: %+v", query.Variables)
			return http.StatusBadRequest, `{}`
		}
	})
	snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Threads) != 101 || len(snapshot.Threads[0].Comments) != 201 ||
		snapshot.Threads[100].ID != "T-100" || snapshot.Threads[100].Comments[0].ID != 1100 {
		t.Fatalf("incomplete snapshot: %d threads", len(snapshot.Threads))
	}
	for index, comment := range snapshot.Threads[0].Comments {
		if comment.ID != int64(index+1) {
			t.Fatalf("comment %d = %d: lost chronological/root ordering", index, comment.ID)
		}
	}
	if !reflect.DeepEqual(requests, []string{":", "T-0:comments-100", "T-0:comments-200", ":threads-100"}) {
		t.Fatalf("pagination requests = %v", requests)
	}
}

func TestGetCommentOutcomesRejectsIncompleteData(t *testing.T) {
	comment := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", 0)
	comments := outcomeTestConnection([]string{comment}, 1, false, "comments")
	thread := outcomeTestThread("T", comments)
	threads := outcomeTestConnection([]string{thread}, 1, false, "threads")
	valid := outcomeTestResponse(threads, "OPEN")
	replace := func(old, new string) string {
		t.Helper()
		if !strings.Contains(valid, old) {
			t.Fatalf("fixture does not contain %s", old)
		}
		return strings.Replace(valid, old, new, 1)
	}
	cases := []struct {
		name, body, want string
	}{
		{"invalid JSON", `{`, "decode comment outcomes"},
		{"GraphQL errors", `{"errors":[{"message":"denied"},{"message":"rate limited"}],"data":null}`, "denied; rate limited"},
		{"partial GraphQL errors", strings.TrimSuffix(valid, "}") + `,"errors":[{"message":"partial"}]}`, "GraphQL errors: partial"},
		{"empty GraphQL error", `{"errors":[{}]}`, "GraphQL errors"},
		{"null response", `null`, "missing or inaccessible"},
		{"null data", `{"data":null}`, "missing or inaccessible"},
		{"missing data", `{}`, "missing or inaccessible"},
		{"null repository", `{"data":{"repository":null}}`, "pull request is missing"},
		{"null PR", `{"data":{"repository":{"pullRequest":null}}}`, "pull request is missing"},
		{"unknown PR state", replace(`"state":"OPEN"`, `"state":"UNKNOWN"`), "invalid pull request state"},
		{"missing PR URL", replace(`"url":"https://example.test/pull/7"`, `"url":""`), "pull request metadata"},
		{"null PR author", replace(`{"login":"pr-author","__typename":"User"}`, `null`), "pull request metadata"},
		{"null threads", replace(threads, `null`), "review threads are missing"},
		{"null thread nodes", replace(`"nodes":[`+thread+`]`, `"nodes":null`), "review threads are missing"},
		{"null thread", replace(thread, `null`), "incomplete review thread"},
		{"missing thread ID", replace(`"id":"T"`, `"id":""`), "incomplete review thread"},
		{"missing resolved flag", replace(`"isResolved":false,`, ``), "incomplete review thread"},
		{"missing outdated flag", replace(`"isOutdated":false,`, ``), "incomplete review thread"},
		{"missing resolver", replace(`"resolvedBy":null,`, ``), "incomplete review thread"},
		{"invalid resolver", replace(`"resolvedBy":null`, `"resolvedBy":{}`), "incomplete review thread"},
		{"null comments", replace(comments, `null`), "thread comments are missing"},
		{"null comment nodes", replace(`"nodes":[`+comment+`]`, `"nodes":null`), "thread comments are missing"},
		{"empty thread", replace(comments, outcomeTestConnection(nil, 0, false, "")), "thread comments are missing"},
		{"null comment", replace(comment, `null`), "incomplete review comment"},
		{"missing database ID", replace(`"databaseId":1,`, ``), "incomplete review comment"},
		{"null database ID", replace(`"databaseId":1`, `"databaseId":null`), "incomplete review comment"},
		{"nonpositive database ID", replace(`"databaseId":1`, `"databaseId":-1`), "incomplete review comment"},
		{"null comment author", replace(`{"login":"reviewer","__typename":"User"}`, `null`), "incomplete review comment"},
		{"missing actor type", replace(`{"login":"reviewer","__typename":"User"}`, `{"login":"reviewer"}`), "incomplete review comment"},
		{"empty comment URL", replace(`"url":"https://example.test/comments/1"`, `"url":""`), "incomplete review comment"},
		{"null comment body", replace(`"body":"Comment 1"`, `"body":null`), "incomplete review comment"},
		{"empty path", replace(`"path":"src/file.go"`, `"path":""`), "incomplete review comment"},
		{"missing created time", replace(`"createdAt":"2026-09-01T00:00:01Z",`, ``), "incomplete review comment"},
		{"missing updated time", replace(`"updatedAt":"2026-09-01T00:01:01Z",`, ``), "incomplete review comment"},
		{"invalid time", replace(`"createdAt":"2026-09-01T00:00:01Z"`, `"createdAt":"invalid"`), "decode comment outcomes"},
		{"updated before created", replace(`"updatedAt":"2026-09-01T00:01:01Z"`, `"updatedAt":"2026-09-01T00:00:00Z"`), "incomplete review comment"},
		{"missing reply parent", replace(`"replyTo":null,`, ``), "incomplete review comment"},
		{"invalid reply parent", replace(`"replyTo":null`, `"replyTo":{}`), "invalid review comment reply parent"},
		{"self reply", replace(`"replyTo":null`, `"replyTo":{"databaseId":1}`), "invalid review comment reply parent"},
		{"missing review", replace(`"pullRequestReview"`, `"omittedReview"`), "incomplete review comment"},
		{"null review", replace(`"pullRequestReview":`, `"pullRequestReview":null,"other":`), "incomplete review comment"},
		{"missing review body", replace(`"pullRequestReview":{"body":`, `"pullRequestReview":{"other":`), "incomplete review comment"},
		{"unknown review state", replace(`"state":"COMMENTED"`, `"state":"UNKNOWN"`), "invalid review state"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				return http.StatusOK, test.body
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
				t.Fatalf("returned partial snapshot on failure: %+v", snapshot)
			}
		})
	}
}

func TestGetCommentOutcomesRejectsMalformedPagination(t *testing.T) {
	comment := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", 0)
	thread := outcomeTestThread("T", outcomeTestConnection([]string{comment}, 1, false, "comments"))
	for _, level := range []string{"threads", "comments"} {
		t.Run(level, func(t *testing.T) {
			node, limit := thread, maxOutcomeThreads
			if level == "comments" {
				node, limit = comment, maxOutcomeComments
			}
			normal := outcomeTestConnection([]string{node}, 1, false, "cursor")
			var tooMany []string
			for range 101 {
				tooMany = append(tooMany, node)
			}
			cases := []struct {
				name, connection, want string
			}{
				{"missing total", strings.Replace(normal, `"totalCount":1,`, "", 1), "incomplete pagination"},
				{"negative total", strings.Replace(normal, `"totalCount":1`, `"totalCount":-1`, 1), "incomplete pagination"},
				{"missing page info", strings.Replace(normal, `"pageInfo"`, `"omitted"`, 1), "incomplete pagination"},
				{"null page info", strings.Replace(normal, `"pageInfo":`, `"pageInfo":null,"other":`, 1), "incomplete pagination"},
				{"missing has next", strings.Replace(normal, `"hasNextPage":false,`, "", 1), "incomplete pagination"},
				{"missing cursor", strings.Replace(normal, `,"endCursor":"cursor"`, "", 1), "missing pagination cursor"},
				{"empty cursor", strings.Replace(normal, `"endCursor":"cursor"`, `"endCursor":""`, 1), "missing pagination cursor"},
				{"null cursor with next", outcomeTestConnection([]string{node}, 2, true, ""), "missing pagination cursor"},
				{"early end", outcomeTestConnection([]string{node}, 2, false, "cursor"), "missing nodes"},
				{"too many nodes", outcomeTestConnection([]string{node}, 0, false, "cursor"), "inconsistent pagination count"},
				{"extra next page", outcomeTestConnection([]string{node}, 1, true, "cursor"), "beyond total count"},
				{"limit exceeded", outcomeTestConnection([]string{node}, limit+1, true, "cursor"), "limit exceeded"},
				{"oversized page", outcomeTestConnection(tooMany, 101, false, "cursor"), "limit exceeded"},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					connection := test.connection
					if level == "comments" {
						connection = outcomeTestConnection([]string{outcomeTestThread("T", connection)}, 1, false, "threads")
					}
					client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
						return http.StatusOK, outcomeTestResponse(connection, "OPEN")
					})
					snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
					if err == nil || !strings.Contains(err.Error(), test.want) {
						t.Fatalf("error = %v, want %q", err, test.want)
					}
					if !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
						t.Fatalf("returned partial snapshot: %+v", snapshot)
					}
				})
			}
		})
	}
}

func TestGetCommentOutcomesRejectsCorruptLaterPages(t *testing.T) {
	marker := review.ReviewMarker(7, "head")
	root := outcomeTestComment(1, "reviewer", "User", marker, "COMMENTED", 0)
	reply := outcomeTestComment(2, "pr-author", "User", "", "COMMENTED", 1)
	for _, level := range []string{"threads", "comments", "ignored comments"} {
		t.Run(level, func(t *testing.T) {
			firstComment := root
			if level == "ignored comments" {
				firstComment = strings.Replace(root, `"login":"reviewer"`, `"login":"human"`, 1)
			}
			firstNode := outcomeTestThread("T1", outcomeTestConnection([]string{firstComment}, 1, false, "comments"))
			secondNode := outcomeTestThread("T2", outcomeTestConnection([]string{reply}, 1, false, "comments"))
			nested := level != "threads"
			if nested {
				firstNode, secondNode = firstComment, reply
			}
			firstPage := outcomeTestConnection([]string{firstNode}, 2, true, "first")
			if nested {
				firstPage = outcomeTestConnection([]string{
					outcomeTestThread("T1", firstPage),
				}, 1, false, "threads")
			}
			normal := outcomeTestConnection([]string{secondNode}, 2, false, "last")
			emptyError := "inconsistent pagination count"
			if nested {
				emptyError = "thread comments are missing"
			}
			cases := []struct {
				name, page, want string
			}{
				{"repeated cursor", strings.Replace(normal, `"last"`, `"first"`, 1), "repeated pagination cursor"},
				{"missing cursor", strings.Replace(normal, `,"endCursor":"last"`, "", 1), "missing pagination cursor"},
				{"changed total", strings.Replace(normal, `"totalCount":2`, `"totalCount":3`, 1), "total changed"},
				{"empty page", outcomeTestConnection(nil, 2, false, ""), emptyError},
				{"duplicate node", outcomeTestConnection([]string{firstNode}, 2, false, "last"), "duplicate review"},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					requests := 0
					client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
						requests++
						if requests == 1 {
							return http.StatusOK, outcomeTestResponse(firstPage, "OPEN")
						}
						if requests != 2 || query.Variables.After == nil || *query.Variables.After != "first" {
							t.Errorf("unexpected pagination request %+v", query.Variables)
							return http.StatusBadRequest, `{}`
						}
						if nested {
							if query.Variables.ThreadID != "T1" {
								t.Errorf("thread ID = %q", query.Variables.ThreadID)
							}
							return http.StatusOK, outcomeTestNodeResponse("T1", test.page)
						}
						return http.StatusOK, outcomeTestResponse(test.page, "OPEN")
					})
					snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
					if err == nil || !strings.Contains(err.Error(), test.want) || requests != 2 {
						t.Fatalf("error = %v, requests = %d; want %q after two requests", err, requests, test.want)
					}
					if !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
						t.Fatalf("returned partial snapshot: %+v", snapshot)
					}
				})
			}
		})
	}
}

func TestGetCommentOutcomesRejectsInaccessibleLaterComments(t *testing.T) {
	root := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", 0)
	first := outcomeTestResponse(outcomeTestConnection([]string{
		outcomeTestThread("T", outcomeTestConnection([]string{root}, 2, true, "first")),
	}, 1, false, "threads"), "OPEN")
	for _, body := range []string{
		`{"data":{"node":null}}`,
		`{"data":{"node":{}}}`,
		`{"data":{"node":{"id":"wrong"}}}`,
		`{"data":{"node":{"id":"T","comments":null}}}`,
		`{"errors":[{"message":"inaccessible comments"}],"data":{"node":null}}`,
	} {
		t.Run(body, func(t *testing.T) {
			requests := 0
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				requests++
				if requests == 1 {
					return http.StatusOK, first
				}
				return http.StatusOK, body
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err == nil || !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) || requests != 2 {
				t.Fatalf("snapshot = %+v, error = %v, requests = %d", snapshot, err, requests)
			}
		})
	}
}

func TestGetCommentOutcomesPreservesPublishedReviewStatesAndNumericIDs(t *testing.T) {
	for _, state := range []string{"COMMENTED", "APPROVED", "CHANGES_REQUESTED", "DISMISSED"} {
		t.Run(state, func(t *testing.T) {
			const id int64 = 9007199254740993
			comment := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), state, 0)
			comment = strings.Replace(comment, `"databaseId":1`, fmt.Sprintf(`"databaseId":%d`, id), 1)
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				return http.StatusOK, outcomeTestResponse(outcomeTestConnection([]string{
					outcomeTestThread("T", outcomeTestConnection([]string{comment}, 1, false, "comment")),
				}, 1, false, "threads"), "OPEN")
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Threads) != 1 || snapshot.Threads[0].Comments[0].ID != id {
				t.Fatalf("snapshot = %+v", snapshot)
			}
		})
	}
}

func TestGetCommentOutcomesRejectsCorruptConversations(t *testing.T) {
	root := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", 0)
	reply := outcomeTestComment(2, "pr-author", "User", "", "COMMENTED", 1)
	cases := []struct {
		name, comment, want string
	}{
		{"second root", strings.Replace(reply, `"replyTo":{"databaseId":1}`, `"replyTo":null`, 1), "does not reply to the thread root"},
		{"unrelated reply", strings.Replace(reply, `"replyTo":{"databaseId":1}`, `"replyTo":{"databaseId":999}`, 1), "does not reply to the thread root"},
		{"backwards time", strings.Replace(reply, `"createdAt":"2026-09-01T00:00:02Z"`, `"createdAt":"2026-09-01T00:00:00Z"`, 1), "chronological order"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				return http.StatusOK, outcomeTestResponse(outcomeTestConnection([]string{
					outcomeTestThread("T", outcomeTestConnection([]string{root, test.comment}, 2, false, "comments")),
				}, 1, false, "threads"), "OPEN")
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
				t.Fatalf("snapshot = %+v, error = %v; want %q", snapshot, err, test.want)
			}
		})
	}
}

func TestGetCommentOutcomesRejectsPaginationCycles(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			requests := 0
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				requests++
				if requests > 3 {
					t.Error("continued after a pagination cycle")
					return http.StatusBadRequest, `{}`
				}
				parent := int64(0)
				if nested && requests > 1 {
					parent = 1
				}
				comment := outcomeTestComment(int64(requests), "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", parent)
				cursor := "first"
				if requests == 2 {
					cursor = "second"
				}
				if nested {
					connection := outcomeTestConnection([]string{comment}, 3, requests < 3, cursor)
					if requests > 1 {
						return http.StatusOK, outcomeTestNodeResponse("T", connection)
					}
					return http.StatusOK, outcomeTestResponse(outcomeTestConnection([]string{
						outcomeTestThread("T", connection),
					}, 1, false, "threads"), "OPEN")
				}
				thread := outcomeTestThread(fmt.Sprintf("T%d", requests), outcomeTestConnection([]string{comment}, 1, false, "comment"))
				return http.StatusOK, outcomeTestResponse(outcomeTestConnection([]string{thread}, 3, requests < 3, cursor), "OPEN")
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err == nil || !strings.Contains(err.Error(), "repeated pagination cursor") ||
				requests != 3 || !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
				t.Fatalf("snapshot = %+v, error = %v, requests = %d", snapshot, err, requests)
			}
		})
	}
}

func TestGetCommentOutcomesRejectsChangingPullRequest(t *testing.T) {
	root := outcomeTestComment(1, "reviewer", "User", review.ReviewMarker(7, "head"), "COMMENTED", 0)
	thread := outcomeTestThread("T", outcomeTestConnection([]string{root}, 1, false, "comments"))
	first := outcomeTestResponse(outcomeTestConnection([]string{thread}, 2, true, "first"), "OPEN")
	for _, second := range []string{
		strings.Replace(first, `"state":"OPEN"`, `"state":"MERGED"`, 1),
		strings.Replace(first, `"login":"pr-author"`, `"login":"someone-else"`, 1),
		strings.Replace(first, `"url":"https://example.test/pull/7"`, `"url":"https://example.test/pull/8"`, 1),
	} {
		t.Run(second, func(t *testing.T) {
			requests := 0
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				requests++
				if requests == 1 {
					return http.StatusOK, first
				}
				return http.StatusOK, second
			})
			snapshot, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
			if err == nil || !strings.Contains(err.Error(), "metadata changed") ||
				requests != 2 || !reflect.DeepEqual(snapshot, outcomes.Snapshot{}) {
				t.Fatalf("snapshot = %+v, error = %v, requests = %d", snapshot, err, requests)
			}
		})
	}
}

func TestOutcomeReadBoundsRequestsAndResponseBytes(t *testing.T) {
	body := outcomeTestResponse(outcomeTestConnection(nil, 0, false, ""), "OPEN")
	for _, test := range []struct {
		name, body, want string
		requests, bytes  int
	}{
		{name: "request budget", body: body, requests: maxOutcomeRequests, want: "request limit exceeded"},
		{name: "total byte budget", body: body, bytes: maxOutcomeBytes, want: "response byte limit exceeded"},
		{name: "individual response cap", body: body + strings.Repeat(" ", 10<<20), want: "response byte limit exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
				calls++
				return http.StatusOK, test.body
			})
			read := outcomeRead{client: client, requests: test.requests, bytes: test.bytes}
			_, err := read.query(context.Background(), outcomeQueryRequest{
				Query: outcomeThreadsQuery, Variables: outcomeQueryVariables{Owner: "owner", Repo: "repo", Number: 7},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if test.requests == maxOutcomeRequests && calls != 0 {
				t.Fatal("exceeded the request budget")
			}
		})
	}
}

func TestOutcomeReadBoundsTotalCommentsAcrossThreads(t *testing.T) {
	read := outcomeRead{commentIDs: make(map[int64]bool)}
	for id := int64(2); id <= maxOutcomeComments+1; id++ {
		read.commentIDs[id] = true
	}
	var comment outcomeComment
	if err := json.Unmarshal([]byte(outcomeTestComment(1, "reviewer", "User", "", "COMMENTED", 0)), &comment); err != nil {
		t.Fatal(err)
	}
	if err := read.validateComment(&comment); err == nil || !strings.Contains(err.Error(), "total comment limit exceeded") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetCommentOutcomesPropagatesHTTPAndAuthenticationFailures(t *testing.T) {
	t.Run("GraphQL HTTP error", func(t *testing.T) {
		client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
			return http.StatusForbidden, `{"message":"forbidden"}`
		})
		_, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusForbidden {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing authenticated identity", func(t *testing.T) {
		client, _ := outcomeTestClient(t, "", func(query outcomeQueryRequest) (int, string) {
			t.Error("queried GraphQL without an authenticated identity")
			return http.StatusOK, `{}`
		})
		if _, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7); err == nil ||
			!strings.Contains(err.Error(), "no login") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("authentication HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodGet || request.URL.Path != "/user" {
				t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			}
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
		}))
		defer server.Close()
		client := NewClientWithBaseURL(server.Client(), "token", server.URL)
		_, err := client.GetCommentOutcomes(context.Background(), "owner", "repo", 7)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusUnauthorized {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		client, _ := outcomeTestClient(t, "reviewer", func(query outcomeQueryRequest) (int, string) {
			t.Error("request was sent despite cancellation")
			return http.StatusOK, `{}`
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := client.GetCommentOutcomes(ctx, "owner", "repo", 7); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}
