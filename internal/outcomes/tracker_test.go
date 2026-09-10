package outcomes

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)

func testThread() Thread {
	return Thread{ID: "thread-1", Comments: []Comment{
		{ID: 1, Author: "reviewer", Body: "This adds repeated work.", URL: "https://github.com/o/r/pull/7#discussion_r1"},
		{ID: 2, Author: "owner", Body: "I disagree; this is already cached.", URL: "https://github.com/o/r/pull/7#discussion_r2"},
	}}
}

func testAssessment() Assessment {
	return Assessment{Feedback: "disagreed", Confidence: .95, Rationale: "Author says it is already cached.",
		Evidence: []Evidence{{CommentID: 2, Quote: "I disagree"}}}
}

func TestOutcomeAndFeedbackAreIndependent(t *testing.T) {
	for _, test := range []struct {
		name, state, outcome, feedback                        string
		resolved, outdated, noReply, bot, unavailable, failed bool
	}{
		{name: "pushback", state: "OPEN", outcome: "replied", feedback: "disagreed"},
		{name: "resolved pushback", state: "MERGED", resolved: true, outcome: "resolved", feedback: "disagreed"},
		{name: "waiting", state: "OPEN", noReply: true, outcome: "pending", feedback: "no-author-reply"},
		{name: "ignored merged", state: "MERGED", noReply: true, outcome: "ignored", feedback: "no-author-reply"},
		{name: "ignored closed", state: "CLOSED", noReply: true, outcome: "ignored", feedback: "no-author-reply"},
		{name: "outdated open", state: "OPEN", noReply: true, outdated: true, outcome: "pending", feedback: "no-author-reply"},
		{name: "resolved silent", state: "CLOSED", noReply: true, resolved: true, outcome: "resolved", feedback: "no-author-reply"},
		{name: "bot not human", state: "MERGED", bot: true, outcome: "ignored", feedback: "no-author-reply"},
		{name: "missing", state: "MERGED", unavailable: true, outcome: "unavailable", feedback: "unavailable"},
		{name: "failed refresh", state: "MERGED", failed: true, outcome: "unavailable", feedback: "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pull := PullRecord{Author: "owner", State: test.state}
			if test.failed {
				pull.Error = "rate limited"
			}
			assessment := testAssessment()
			record := CommentRecord{Available: !test.unavailable, Thread: testThread(), Assessment: &assessment}
			record.Thread.Resolved, record.Thread.Outdated = test.resolved, test.outdated
			if test.noReply {
				record.Thread.Comments = record.Thread.Comments[:1]
			}
			if test.bot {
				record.Thread.Comments[1].Bot = true
			}
			if got := Outcome(pull, record); got != test.outcome {
				t.Fatalf("outcome = %s", got)
			}
			if got := Feedback(pull, record); got != test.feedback {
				t.Fatalf("feedback = %s", got)
			}
		})
	}
}

func TestFeedbackConfidenceAndOtherParticipants(t *testing.T) {
	pull := PullRecord{Author: "owner", State: "OPEN"}
	assessment := testAssessment()
	record := CommentRecord{Available: true, Thread: testThread(), Assessment: &assessment}
	assessment.Confidence = ConfidenceThreshold - .001
	if Feedback(pull, record) != "unclear" {
		t.Fatal("low confidence counted as disagreement")
	}
	assessment.Confidence = ConfidenceThreshold
	if Feedback(pull, record) != "disagreed" {
		t.Fatal("threshold should be inclusive")
	}
	record.Thread.Comments[1].Author = "other-maintainer"
	if Outcome(pull, record) != "replied" || Feedback(pull, record) != "no-author-reply" {
		t.Fatal("other reply mistaken for author")
	}
	record.Thread.Comments[1].Author = "reviewer"
	if Outcome(pull, record) != "pending" {
		t.Fatal("reviewer's own reply counted")
	}
}

func TestValidateAssessment(t *testing.T) {
	input := Input{Author: "OWNER", Comments: testThread().Comments}
	if err := ValidateAssessment(input, testAssessment()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Assessment){
		func(a *Assessment) { a.Feedback = "fixed" },
		func(a *Assessment) { a.Confidence = math.NaN() },
		func(a *Assessment) { a.Confidence = math.Inf(1) },
		func(a *Assessment) { a.Confidence = 1.01 },
		func(a *Assessment) { a.Confidence = -.1 },
		func(a *Assessment) { a.Rationale = "" },
		func(a *Assessment) { a.Evidence = nil },
		func(a *Assessment) { a.Evidence[0].CommentID = 1 },
		func(a *Assessment) { a.Evidence[0].CommentID = 999 },
		func(a *Assessment) { a.Evidence[0].Quote = "invented" },
		func(a *Assessment) { a.Evidence[0].Quote = "" },
	} {
		a := testAssessment()
		mutate(&a)
		if err := ValidateAssessment(input, a); err == nil {
			t.Fatalf("accepted invalid assessment: %+v", a)
		}
	}
	input.Comments[1].Bot = true
	if err := ValidateAssessment(input, testAssessment()); err == nil {
		t.Fatal("accepted bot evidence")
	}
}

type fakeReader struct {
	snapshot Snapshot
	err      error
	calls    []int
}

func (reader *fakeReader) GetCommentOutcomes(_ context.Context, _, _ string, number int) (Snapshot, error) {
	reader.calls = append(reader.calls, number)
	snapshot := reader.snapshot
	snapshot.Number = number
	return snapshot, reader.err
}

type fakeClassifier struct {
	calls int
	err   error
}

func (classifier *fakeClassifier) Assess(_ context.Context, _ Input) (Assessment, error) {
	classifier.calls++
	return testAssessment(), classifier.err
}

func testOptions() Options {
	return Options{Owner: "o", Repo: "r", Model: "model", MaxPulls: 100, MaxAssessments: 10}
}

func TestRefreshCachesByConversationAndModelAndTracksMissing(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "OPEN", Threads: []Thread{testThread()}}}
	classifier := &fakeClassifier{}
	options := testOptions()
	refresh := func() {
		t.Helper()
		if err := Refresh(context.Background(), &ledger, []int{7, 7}, reader, classifier, options, testNow); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	refresh()
	if len(ledger.Pulls) != 1 || ledger.Statistics.Total != 1 || classifier.calls != 1 {
		t.Fatalf("ledger %+v; calls %d", ledger, classifier.calls)
	}
	reader.snapshot.Threads[0].Resolved = true
	refresh()
	if classifier.calls != 1 || ledger.Statistics.Outcomes["resolved"] != 1 || ledger.Statistics.AuthorFeedback["disagreed"] != 1 {
		t.Fatal("resolution changed semantic assessment")
	}
	reader.snapshot.Threads[0].Comments[1].Body += " But I'll change it."
	refresh()
	options.Model = "new-model"
	refresh()
	options.ReasoningEffort = "high"
	refresh()
	if classifier.calls != 4 {
		t.Fatalf("classification calls = %d", classifier.calls)
	}
	reader.snapshot.Threads = nil
	refresh()
	if ledger.Statistics.Total != 1 || ledger.Statistics.Outcomes["unavailable"] != 1 {
		t.Fatal("lost missing comment")
	}
	reader.snapshot.Threads = []Thread{testThread()}
	reader.snapshot.Threads[0].Comments = reader.snapshot.Threads[0].Comments[:1]
	refresh()
	if ledger.Pulls[0].Comments[0].Assessment != nil || ledger.Statistics.AuthorFeedback["no-author-reply"] != 1 {
		t.Fatal("deleted reply retained old assessment")
	}
}

func TestRefreshPersistsFactsOnFailuresAndRetries(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "OPEN", Threads: []Thread{testThread()}}}
	classifier := &fakeClassifier{err: errors.New("model unavailable")}
	err := Refresh(context.Background(), &ledger, []int{7}, reader, classifier, testOptions(), testNow)
	if err == nil || ledger.Statistics.Outcomes["replied"] != 1 || ledger.Statistics.AuthorFeedback["error"] != 1 {
		t.Fatalf("err %v ledger %+v", err, ledger)
	}
	classifier.err = nil
	if err := Refresh(context.Background(), &ledger, nil, reader, classifier, testOptions(), testNow); err != nil {
		t.Fatal(err)
	}
	if ledger.Statistics.AuthorFeedback["disagreed"] != 1 {
		t.Fatal("classification not retried")
	}
	reader.err = errors.New("GitHub unavailable")
	err = Refresh(context.Background(), &ledger, nil, reader, classifier, testOptions(), testNow.Add(time.Hour))
	if err == nil || ledger.Statistics.Outcomes["unavailable"] != 1 || ledger.Statistics.FailedPulls != 1 {
		t.Fatalf("err %v ledger %+v", err, ledger)
	}
	if !ledger.Pulls[0].LastCheckedAt.Equal(testNow) {
		t.Fatal("failed refresh advanced checked time")
	}
	reader.err = nil
	if err := Refresh(context.Background(), &ledger, nil, reader, classifier, testOptions(), testNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ledger.Statistics.FailedPulls != 0 || ledger.Statistics.AuthorFeedback["disagreed"] != 1 {
		t.Fatal("recovery failed")
	}
}

func TestRefreshBudgetsRotateAndDoNotEvictOldPRs(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "OPEN", Threads: []Thread{testThread()}}}
	classifier := &fakeClassifier{}
	options := testOptions()
	options.MaxPulls, options.MaxAssessments = 2, 1
	if err := Refresh(context.Background(), &ledger, []int{1, 2, 3}, reader, classifier, options, testNow); err != nil {
		t.Fatal(err)
	}
	if classifier.calls != 1 || len(reader.calls) != 2 || ledger.Statistics.AuthorFeedback["awaiting-assessment"] != 1 {
		t.Fatalf("budget mismatch: %+v", ledger)
	}
	if err := Refresh(context.Background(), &ledger, nil, reader, classifier, options, testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if reader.calls[2] != 3 || len(ledger.Pulls) != 3 {
		t.Fatalf("did not rotate: %v", reader.calls)
	}
}

func TestRefreshRejectsWrongRepositoryAndLimits(t *testing.T) {
	for _, test := range []struct {
		repository string
		limit      int
		expected   string
	}{
		{"wrong/repo", 100, "repository"}, {"o/r", 0, "limits"},
	} {
		ledger := Ledger{Version: Version, Repository: test.repository}
		options := testOptions()
		options.MaxPulls = test.limit
		if err := Refresh(context.Background(), &ledger, nil, nil, nil, options, testNow); err == nil || !strings.Contains(err.Error(), test.expected) {
			t.Fatalf("err %v", err)
		}

	}
}

func TestFailedAssessmentDoesNotStarveOtherThreads(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	second := testThread()
	second.ID = "thread-2"
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "OPEN", Threads: []Thread{testThread(), second}}}
	classifier := &fakeClassifier{err: errors.New("model error")}
	options := testOptions()
	options.MaxAssessments = 1
	if err := Refresh(context.Background(), &ledger, []int{7}, reader, classifier, options, testNow); err == nil {
		t.Fatal("expected assessment error")
	}
	classifier.err = nil
	if err := Refresh(context.Background(), &ledger, nil, reader, classifier, options, testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ledger.Pulls[0].Comments[0].Assessment != nil || ledger.Pulls[0].Comments[1].Assessment == nil {
		t.Fatal("failed assessment starved an unassessed thread")
	}
}

func TestChangedConversationDoesNotKeepStaleFeedbackOnFailure(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "OPEN", Threads: []Thread{testThread()}}}
	classifier := &fakeClassifier{}
	if err := Refresh(context.Background(), &ledger, []int{7}, reader, classifier, testOptions(), testNow); err != nil {
		t.Fatal(err)
	}
	reader.snapshot.Threads[0].Comments[1].Body = "Actually, thanks. Fixed."
	classifier.err = errors.New("assessment unavailable")
	if err := Refresh(context.Background(), &ledger, nil, reader, classifier, testOptions(), testNow.Add(time.Hour)); err == nil {
		t.Fatal("expected assessment error")
	}
	if ledger.Pulls[0].Comments[0].Assessment != nil ||
		ledger.Statistics.AuthorFeedback["disagreed"] != 0 ||
		ledger.Statistics.AuthorFeedback["error"] != 1 {
		t.Fatal("old disagreement counted after a changed reply")
	}
}

func TestReopenedAndUnresolvedThreadsAreRefreshed(t *testing.T) {
	ledger := Ledger{Version: Version, Repository: "o/r"}
	thread := testThread()
	thread.Comments = thread.Comments[:1]
	reader := &fakeReader{snapshot: Snapshot{Author: "owner", State: "MERGED", Threads: []Thread{thread}}}
	for _, test := range []struct {
		state    string
		resolved bool
		want     string
	}{
		{"MERGED", false, "ignored"},
		{"OPEN", false, "pending"},
		{"OPEN", true, "resolved"},
		{"OPEN", false, "pending"},
	} {
		reader.snapshot.State = test.state
		reader.snapshot.Threads[0].Resolved = test.resolved
		if err := Refresh(context.Background(), &ledger, []int{7}, reader, &fakeClassifier{}, testOptions(), testNow); err != nil {
			t.Fatal(err)
		}
		if ledger.Statistics.Total != 1 || ledger.Statistics.Outcomes[test.want] != 1 {
			t.Fatalf("transition to %s resolved=%t: %+v", test.state, test.resolved, ledger.Statistics)
		}
	}
}
