package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/github"
	"github.com/roblourens/rob-reviewer/internal/learning"
)

const learnedGuidancePath = "reviewers/performance/references/learned-regressions.md"

func PublishLearnedGuidance(
	ctx context.Context,
	repository, branch, proposalsPath, casesPath, token string,
) (bool, error) {
	owner, repo, err := parseRepository(repository)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(branch) == "" {
		return false, errors.New("learning target branch is required")
	}
	if strings.TrimSpace(token) == "" {
		return false, errors.New("GITHUB_TOKEN is required to publish learned guidance")
	}
	proposals, err := loadLearningProposals(proposalsPath)
	if err != nil {
		return false, err
	}
	if len(proposals) == 0 {
		return false, nil
	}
	cases, err := loadRegressionCases(casesPath)
	if err != nil {
		return false, err
	}
	if err := validateLearningProposalBindings(proposals, cases); err != nil {
		return false, err
	}
	client := github.NewClient(nil, token)
	for attempt := 0; attempt < 3; attempt++ {
		current, exists, err := client.GetContent(ctx, owner, repo, learnedGuidancePath, branch)
		if err != nil {
			return false, fmt.Errorf("load learned performance guidance: %w", err)
		}
		if !exists {
			return false, fmt.Errorf("learned performance guidance %q does not exist on %s", learnedGuidancePath, branch)
		}
		updated, changed, err := learning.ApplyProposals(string(current.Content), proposals)
		if err != nil {
			return false, err
		}
		if !changed {
			return false, nil
		}
		err = client.PutContent(
			ctx,
			owner,
			repo,
			learnedGuidancePath,
			branch,
			"Learn from shipped performance regression (Written by Copilot)\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
			[]byte(updated),
			current.SHA,
		)
		if err == nil {
			return true, nil
		}
		var apiError *github.APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != 409 {
			return false, fmt.Errorf("publish learned performance guidance: %w", err)
		}
	}
	return false, errors.New("publish learned performance guidance: concurrent updates did not settle after three attempts")
}

func loadRegressionCases(path string) ([]learning.RegressionCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open regression cases: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSavedReportBytes {
		return nil, fmt.Errorf("regression cases %q must be a regular file no larger than %d bytes", path, maxSavedReportBytes)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var evaluation learning.Evaluation
	if err := decoder.Decode(&evaluation); err != nil {
		return nil, fmt.Errorf("decode regression cases: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("regression cases %q contain trailing JSON", path)
	}
	return evaluation.Cases, nil
}

func validateLearningProposalBindings(proposals []learning.Proposal, cases []learning.RegressionCase) error {
	caseByID := make(map[string]learning.RegressionCase, len(cases))
	for _, regressionCase := range cases {
		if regressionCase.ID == "" {
			return errors.New("regression case ID is required")
		}
		if _, duplicate := caseByID[regressionCase.ID]; duplicate {
			return fmt.Errorf("duplicate regression case ID %q", regressionCase.ID)
		}
		caseByID[regressionCase.ID] = regressionCase
	}
	for _, proposal := range proposals {
		regressionCase, exists := caseByID[proposal.CaseID]
		if !exists {
			return fmt.Errorf("learning proposal %q has no matching current-run regression case", proposal.CaseID)
		}
		if regressionCase.Coverage.Status != "instruction-gap" {
			return fmt.Errorf("learning proposal %q is not backed by an instruction gap", proposal.CaseID)
		}
		if regressionCase.MechanismFamily != proposal.MechanismFamily {
			return fmt.Errorf("learning proposal %q mechanism family does not match its regression case", proposal.CaseID)
		}
	}
	return nil
}

func loadLearningProposals(root string) ([]learning.Proposal, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat learning proposal directory: %w", err)
	}
	if info.Mode().IsRegular() {
		return readLearningProposalFile(root)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("learning proposal path %q is neither a regular file nor a directory", root)
	}
	var proposals []learning.Proposal
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "learning-proposals.json" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("learning proposals %q cannot be a symbolic link", path)
		}
		fileProposals, err := readLearningProposalFile(path)
		if err != nil {
			return err
		}
		proposals = append(proposals, fileProposals...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load learning proposals: %w", err)
	}
	return proposals, nil
}

func readLearningProposalFile(path string) ([]learning.Proposal, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSavedReportBytes {
		_ = file.Close()
		return nil, fmt.Errorf("learning proposals %q must be a regular file no larger than %d bytes", path, maxSavedReportBytes)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var proposals []learning.Proposal
	if err := decoder.Decode(&proposals); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("decode learning proposals %q: %w", path, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		_ = file.Close()
		return nil, fmt.Errorf("learning proposals %q contain trailing JSON", path)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return proposals, nil
}
