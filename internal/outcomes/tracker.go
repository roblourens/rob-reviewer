package outcomes

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	Version             = 1
	AssessmentVersion   = "author-feedback-v1"
	ConfidenceThreshold = 0.8
)

type Ledger struct {
	Version    int          `json:"version"`
	Repository string       `json:"repository"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	Pulls      []PullRecord `json:"pulls"`
	Statistics Statistics   `json:"statistics"`
}

type PullRecord struct {
	Number        int             `json:"number"`
	URL           string          `json:"url"`
	Author        string          `json:"author"`
	State         string          `json:"state"`
	LastAttemptAt time.Time       `json:"lastAttemptAt"`
	LastCheckedAt time.Time       `json:"lastCheckedAt"`
	Error         string          `json:"error,omitempty"`
	Comments      []CommentRecord `json:"comments"`
}

type CommentRecord struct {
	Thread              Thread      `json:"thread"`
	Available           bool        `json:"available"`
	LastSeenAt          time.Time   `json:"lastSeenAt"`
	Assessment          *Assessment `json:"assessment,omitempty"`
	AssessmentKey       string      `json:"assessmentKey,omitempty"`
	AssessmentModel     string      `json:"assessmentModel,omitempty"`
	AssessedAt          *time.Time  `json:"assessedAt,omitempty"`
	AssessmentAttemptAt time.Time   `json:"assessmentAttemptAt"`
	AssessmentError     string      `json:"assessmentError,omitempty"`
}

type Statistics struct {
	Total               int            `json:"total"`
	Outcomes            map[string]int `json:"outcomes"`
	AuthorFeedback      map[string]int `json:"authorFeedback"`
	FailedPulls         int            `json:"failedPulls"`
	ConfidenceThreshold float64        `json:"confidenceThreshold"`
}

type Options struct {
	Owner           string
	Repo            string
	Model           string
	ReasoningEffort string
	MaxPulls        int
	MaxAssessments  int
}

func Refresh(ctx context.Context, ledger *Ledger, numbers []int, reader Reader, classifier Classifier, options Options, now time.Time) error {
	if options.MaxPulls <= 0 || options.MaxAssessments <= 0 {
		return errors.New("outcome refresh limits must be positive")
	}
	repository := options.Owner + "/" + options.Repo
	if ledger.Version != Version || !strings.EqualFold(ledger.Repository, repository) {
		return errors.New("outcome ledger version or repository does not match")
	}
	for _, number := range numbers {
		if number <= 0 {
			return errors.New("outcome pull request numbers must be positive")
		}
	}
	for _, number := range numbers {
		if !slices.ContainsFunc(ledger.Pulls, func(pull PullRecord) bool { return pull.Number == number }) {
			ledger.Pulls = append(ledger.Pulls, PullRecord{Number: number, Comments: []CommentRecord{}})
		}
	}
	slices.SortFunc(ledger.Pulls, func(a, b PullRecord) int {
		if order := a.LastAttemptAt.Compare(b.LastAttemptAt); order != 0 {
			return order
		}
		return a.Number - b.Number
	})
	now = now.UTC()
	remaining := options.MaxAssessments
	var failures []error
	for index := 0; index < len(ledger.Pulls) && index < options.MaxPulls; index++ {
		pull := &ledger.Pulls[index]
		pull.LastAttemptAt = now
		snapshot, err := reader.GetCommentOutcomes(ctx, options.Owner, options.Repo, pull.Number)
		if err != nil {
			pull.Error = err.Error()
			failures = append(failures, fmt.Errorf("refresh PR %d outcomes: %w", pull.Number, err))
			continue
		}
		pull.Error = ""
		pull.URL, pull.Author, pull.State = snapshot.URL, snapshot.Author, snapshot.State
		pull.LastCheckedAt = now
		for i := range pull.Comments {
			pull.Comments[i].Available = false
		}
		for _, thread := range snapshot.Threads {
			existing := slices.IndexFunc(pull.Comments, func(record CommentRecord) bool { return record.Thread.ID == thread.ID })
			if existing < 0 {
				pull.Comments = append(pull.Comments, CommentRecord{})
				existing = len(pull.Comments) - 1
			}
			record := &pull.Comments[existing]
			record.Thread, record.Available, record.LastSeenAt = thread, true, now
		}
		slices.SortStableFunc(pull.Comments, func(a, b CommentRecord) int {
			return a.AssessmentAttemptAt.Compare(b.AssessmentAttemptAt)
		})
		for i := range pull.Comments {
			record := &pull.Comments[i]
			if !record.Available {
				continue
			}
			input := Input{Author: pull.Author, Comments: record.Thread.Comments}
			key, err := assessmentKey(input, options)
			if err != nil {
				return err
			}
			if record.AssessmentKey != key {
				record.Assessment = nil
				record.AssessedAt = nil
				record.AssessmentModel = ""
				record.AssessmentError = ""
				record.AssessmentKey = key
			}
			if record.Assessment != nil || !hasAuthorReply(input) || remaining == 0 {
				continue
			}
			remaining--
			record.AssessmentAttemptAt = now
			assessment, err := classifier.Assess(ctx, input)
			if err == nil {
				err = ValidateAssessment(input, assessment)
			}
			if err != nil {
				record.AssessmentError = err.Error()
				failures = append(failures, fmt.Errorf("assess PR %d thread %s: %w", pull.Number, record.Thread.ID, err))
				continue
			}
			record.Assessment = &assessment
			record.AssessmentModel = options.Model
			record.AssessedAt = &now
			record.AssessmentError = ""
		}
		slices.SortFunc(pull.Comments, func(a, b CommentRecord) int {
			return strings.Compare(a.Thread.ID, b.Thread.ID)
		})
	}
	slices.SortFunc(ledger.Pulls, func(a, b PullRecord) int { return a.Number - b.Number })
	ledger.UpdatedAt = now
	ledger.Statistics = Summarize(*ledger)
	return errors.Join(failures...)
}

func assessmentKey(input Input, options Options) (string, error) {
	content, err := json.Marshal(struct {
		Version string `json:"version"`
		Model   string `json:"model"`
		Effort  string `json:"effort"`
		Input   Input  `json:"input"`
	}{AssessmentVersion, options.Model, options.ReasoningEffort, input})
	if err != nil {
		return "", fmt.Errorf("encode feedback input: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(content)), nil
}

func hasAuthorReply(input Input) bool {
	return slices.ContainsFunc(input.Comments, func(comment Comment) bool { return IsAuthorReply(input, comment) })
}

func Outcome(pull PullRecord, record CommentRecord) string {
	if !record.Available || pull.Error != "" {
		return "unavailable"
	}
	if record.Thread.Resolved {
		return "resolved"
	}
	if len(record.Thread.Comments) == 0 {
		return "unavailable"
	}
	root := record.Thread.Comments[0]
	for _, reply := range record.Thread.Comments[1:] {
		if !reply.Bot && reply.Author != "" && !strings.EqualFold(reply.Author, root.Author) {
			return "replied"
		}
	}
	if pull.State == "CLOSED" || pull.State == "MERGED" {
		return "ignored"
	}
	return "pending"
}

func Feedback(pull PullRecord, record CommentRecord) string {
	if !record.Available || pull.Error != "" {
		return "unavailable"
	}
	if !hasAuthorReply(Input{Author: pull.Author, Comments: record.Thread.Comments}) {
		return "no-author-reply"
	}
	if record.AssessmentError != "" {
		return "error"
	}
	if record.Assessment == nil {
		return "awaiting-assessment"
	}
	if record.Assessment.Confidence < ConfidenceThreshold {
		return "unclear"
	}
	return record.Assessment.Feedback
}

func Summarize(ledger Ledger) Statistics {
	result := Statistics{
		Outcomes: map[string]int{"resolved": 0, "replied": 0, "ignored": 0, "pending": 0, "unavailable": 0},
		AuthorFeedback: map[string]int{
			"accepted": 0, "disagreed": 0, "mixed": 0, "unclear": 0,
			"no-author-reply": 0, "awaiting-assessment": 0, "error": 0, "unavailable": 0,
		},
		ConfidenceThreshold: ConfidenceThreshold,
	}
	for _, pull := range ledger.Pulls {
		if pull.Error != "" {
			result.FailedPulls++
		}
		for _, record := range pull.Comments {
			result.Total++
			result.Outcomes[Outcome(pull, record)]++
			result.AuthorFeedback[Feedback(pull, record)]++
		}
	}
	return result
}
