package outcomes

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Snapshot contains complete, paginated GitHub thread data for published reviewer comments.
type Snapshot struct {
	Number  int      `json:"number"`
	URL     string   `json:"url"`
	Author  string   `json:"author"`
	State   string   `json:"state"`
	Threads []Thread `json:"threads"`
}

type Thread struct {
	ID         string    `json:"id"`
	Resolved   bool      `json:"resolved"`
	Outdated   bool      `json:"outdated"`
	ResolvedBy string    `json:"resolvedBy,omitempty"`
	Comments   []Comment `json:"comments"`
}

type Comment struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Author    string    `json:"author"`
	Bot       bool      `json:"bot"`
	Body      string    `json:"body"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Reader interface {
	GetCommentOutcomes(ctx context.Context, owner, repo string, number int) (Snapshot, error)
}

type Evidence struct {
	CommentID int64  `json:"commentId"`
	Quote     string `json:"quote"`
}

type Assessment struct {
	Feedback   string     `json:"feedback" jsonschema:"accepted, disagreed, mixed, or unclear; describes the PR author's replies, not whether code was fixed"`
	Confidence float64    `json:"confidence" jsonschema:"Confidence from 0 through 1"`
	Rationale  string     `json:"rationale" jsonschema:"Short explanation grounded only in the supplied conversation"`
	Evidence   []Evidence `json:"evidence" jsonschema:"Exact quotes from PR-author replies supporting the assessment"`
}

type Input struct {
	Author   string    `json:"prAuthor"`
	Comments []Comment `json:"comments"`
}

type Classifier interface {
	Assess(context.Context, Input) (Assessment, error)
}

func IsAuthorReply(input Input, comment Comment) bool {
	return !comment.Bot && comment.Author != "" &&
		strings.EqualFold(comment.Author, input.Author) &&
		len(input.Comments) > 0 && comment.ID != input.Comments[0].ID &&
		!strings.EqualFold(comment.Author, input.Comments[0].Author)
}

func ValidateAssessment(input Input, assessment Assessment) error {
	switch assessment.Feedback {
	case "accepted", "disagreed", "mixed", "unclear":
	default:
		return fmt.Errorf("invalid author feedback %q", assessment.Feedback)
	}
	if math.IsNaN(assessment.Confidence) || math.IsInf(assessment.Confidence, 0) ||
		assessment.Confidence < 0 || assessment.Confidence > 1 {
		return errors.New("feedback confidence must be between 0 and 1")
	}
	if strings.TrimSpace(assessment.Rationale) == "" || len(assessment.Rationale) > 4000 {
		return errors.New("feedback rationale must contain 1 through 4000 bytes")
	}
	if len(assessment.Evidence) == 0 {
		return errors.New("feedback assessment requires PR-author evidence")
	}
	for _, evidence := range assessment.Evidence {
		valid := false
		for _, comment := range input.Comments {
			if comment.ID == evidence.CommentID && IsAuthorReply(input, comment) &&
				strings.TrimSpace(evidence.Quote) != "" && strings.Contains(comment.Body, evidence.Quote) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("feedback evidence does not match a PR-author reply: %d", evidence.CommentID)
		}
	}
	return nil
}
