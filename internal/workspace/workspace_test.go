package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roblourens/rob-reviewer/internal/review"
)

const (
	testBaseSHA   = "1111111111111111111111111111111111111111"
	testHeadSHA   = "2222222222222222222222222222222222222222"
	testMergeBase = "3333333333333333333333333333333333333333"
)

type recordedCommand struct {
	directory string
	limits    commandLimits
	args      []string
}

type fakeRunner struct {
	commands          []recordedCommand
	failOnSharedClone bool
}

func (runner *fakeRunner) Run(
	_ context.Context,
	directory string,
	limits commandLimits,
	args ...string,
) ([]byte, error) {
	runner.commands = append(runner.commands, recordedCommand{
		directory: directory,
		limits:    limits,
		args:      append([]string(nil), args...),
	})
	if runner.failOnSharedClone && args[0] == "clone" && slices.Contains(args, "--shared") {
		return nil, errors.New("command failed")
	}
	switch args[0] {
	case "init", "config", "remote", "fetch", "clone", "checkout":
		return nil, nil
	case "merge-base":
		return []byte(testMergeBase + "\n"), nil
	case "rev-parse":
		ref := args[len(args)-1]
		switch ref {
		case "--is-bare-repository":
			return []byte("true\n"), nil
		case cachedCommitRef(testBaseSHA) + "^{commit}":
			return []byte(testBaseSHA + "\n"), nil
		case cachedCommitRef(testHeadSHA) + "^{commit}", "HEAD^{commit}":
			return []byte(testHeadSHA + "\n"), nil
		default:
			return nil, errors.New("unexpected ref")
		}
	case "diff":
		return []byte("diff contents\n"), nil
	default:
		return nil, errors.New("unexpected git command")
	}
}

func TestNewUsesPersistentCacheAndCleansOnlyWorkspace(t *testing.T) {
	cacheRoot := testDirectory(t)
	sibling := filepath.Join(cacheRoot, "keep")
	if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	pullRequest := validPullRequest()

	workspace, err := newWithRunner(
		context.Background(),
		"owner",
		"repository",
		pullRequest,
		cacheRoot,
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.MergeBase != testMergeBase || workspace.HeadSHA != testHeadSHA {
		t.Fatalf("unexpected workspace metadata: %+v", workspace)
	}
	if workspace.DiffString() != "diff contents\n" {
		t.Fatalf("diff = %q", workspace.DiffString())
	}

	repositoryCache := filepath.Join(cacheRoot, "repositories", "owner", "repository.git")
	if info, err := os.Stat(repositoryCache); err != nil || !info.IsDir() {
		t.Fatalf("persistent cache was not created: info=%v err=%v", info, err)
	}
	assertCommand(t, runner.commands, []string{
		"clone", "--bare", "--filter=blob:none", "--no-tags", "--quiet", "--",
		"https://github.com/owner/repository.git", ".",
	})
	assertCommand(t, runner.commands, []string{
		"fetch", "--quiet", "--force", "--filter=blob:none", "--no-tags", "--no-write-fetch-head",
		"https://github.com/owner/repository.git",
		"+" + testBaseSHA + ":" + cachedCommitRef(testBaseSHA),
	})
	assertCommand(t, runner.commands, []string{
		"clone", "--shared", "--no-checkout", "--no-tags", "--quiet", "--",
		repositoryCache, ".",
	})
	assertCommand(t, runner.commands, []string{"config", "extensions.partialClone", "origin"})
	assertCommand(t, runner.commands, []string{
		"checkout", "--quiet", "--detach", "--force", testHeadSHA,
	})
	diffCommand := assertCommand(t, runner.commands, []string{
		"diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--unified=3",
		testMergeBase, testHeadSHA, "--",
	})
	if diffCommand.limits.stdout != diffStdoutLimit || diffCommand.limits.stderr != commandStderrLimit {
		t.Fatalf("unexpected diff limits: %+v", diffCommand.limits)
	}

	createdRoot := workspace.Root
	workspace.Root = sibling
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(createdRoot); !os.IsNotExist(err) {
		t.Fatalf("created workspace was not removed: %v", err)
	}
	if _, err := os.Stat(repositoryCache); err != nil {
		t.Fatalf("persistent cache was removed: %v", err)
	}
	if content, err := os.ReadFile(sibling); err != nil || string(content) != "keep" {
		t.Fatalf("sibling was modified: content=%q err=%v", content, err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	workspace, err = newWithRunner(
		context.Background(),
		"owner",
		"repository",
		pullRequest,
		cacheRoot,
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if countCommandsWithArg(runner.commands, "clone", "--bare") != 1 {
		t.Fatalf("repository cache was initialized more than once: %+v", runner.commands)
	}
}

func TestNewCleansWorkspaceAfterCloneFailure(t *testing.T) {
	cacheRoot := testDirectory(t)
	runner := &fakeRunner{failOnSharedClone: true}
	_, err := newWithRunner(
		context.Background(),
		"owner",
		"repo",
		validPullRequest(),
		cacheRoot,
		runner,
	)
	if err == nil || !strings.Contains(err.Error(), "create shared workspace clone") {
		t.Fatalf("expected clone error, got %v", err)
	}
	workspaceParent := filepath.Join(cacheRoot, "workspaces")
	entries, readErr := os.ReadDir(workspaceParent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary workspace was not cleaned up: %+v", entries)
	}
	repositoryCache := filepath.Join(cacheRoot, "repositories", "owner", "repo.git")
	if _, err := os.Stat(repositoryCache); err != nil {
		t.Fatalf("persistent cache should survive workspace failure: %v", err)
	}
}

func TestLimitedBufferBoundsMemory(t *testing.T) {
	buffer := newLimitedBuffer(4)
	written, err := buffer.Write([]byte("abcdef"))
	if !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("expected output limit error, got %v", err)
	}
	if written != 4 || string(buffer.Bytes()) != "abcd" || !buffer.overflow {
		t.Fatalf("unexpected limited buffer state: written=%d content=%q overflow=%t", written, buffer.String(), buffer.overflow)
	}
	written, err = buffer.Write([]byte("more"))
	if !errors.Is(err, ErrOutputLimitExceeded) || written != 0 || len(buffer.Bytes()) != 4 {
		t.Fatalf("overflowing write grew buffer: written=%d content=%q err=%v", written, buffer.String(), err)
	}
}

func TestExecGitRunnerReturnsOutputLimitErrors(t *testing.T) {
	root := testDirectory(t)
	runner := execGitRunner{}
	ctx := context.Background()
	if _, err := runner.Run(ctx, root, defaultCommandLimits, "init", "--quiet", "."); err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(root, "payload")
	if err := os.WriteFile(payloadPath, []byte("large output"), 0o600); err != nil {
		t.Fatal(err)
	}
	hashOutput, err := runner.Run(ctx, root, defaultCommandLimits, "hash-object", "-w", "payload")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run(
		ctx,
		root,
		commandLimits{stdout: 4, stderr: commandStderrLimit},
		"cat-file",
		"blob",
		strings.TrimSpace(string(hashOutput)),
	)
	var limitErr *OutputLimitError
	if !errors.As(err, &limitErr) || limitErr.Stream != "stdout" || limitErr.Limit != 4 {
		t.Fatalf("expected stdout limit error, got %v", err)
	}

	_, err = runner.Run(
		ctx,
		root,
		commandLimits{stdout: commandStdoutLimit, stderr: 4},
		"not-a-command",
	)
	if !errors.As(err, &limitErr) || limitErr.Stream != "stderr" || limitErr.Limit != 4 {
		t.Fatalf("expected stderr limit error, got %v", err)
	}
}

func TestValidateRequestRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		repo  string
		pr    review.PullRequest
	}{
		{name: "owner", owner: "../owner", repo: "repo", pr: validPullRequest()},
		{name: "repo", owner: "owner", repo: "repo?x=1", pr: validPullRequest()},
		{name: "base ref", owner: "owner", repo: "repo", pr: withBaseRef(validPullRequest(), "--upload-pack=x")},
		{name: "head sha", owner: "owner", repo: "repo", pr: withHeadSHA(validPullRequest(), "not-a-sha")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRequest(test.owner, test.repo, test.pr); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCommandErrorsAreUseful(t *testing.T) {
	commandErr := (&CommandError{
		Args:   []string{"fetch", "https://github.com/owner/repo.git"},
		Stderr: "fatal: repository not found",
		Err:    errors.New("exit status 128"),
	}).Error()
	if !strings.Contains(commandErr, "fatal: repository not found") || !strings.Contains(commandErr, `"fetch"`) {
		t.Fatalf("unhelpful command error: %s", commandErr)
	}

	limitErr := &OutputLimitError{
		Args:   []string{"diff"},
		Stream: "stdout",
		Limit:  diffStdoutLimit,
	}
	if !errors.Is(limitErr, ErrOutputLimitExceeded) ||
		!strings.Contains(limitErr.Error(), "stdout exceeded 33554432-byte limit") {
		t.Fatalf("unhelpful output limit error: %v", limitErr)
	}
}

func validPullRequest() review.PullRequest {
	return review.PullRequest{
		Number:  42,
		BaseRef: "main",
		BaseSHA: testBaseSHA,
		HeadRef: "feature/topic",
		HeadSHA: testHeadSHA,
	}
}

func withBaseRef(pullRequest review.PullRequest, ref string) review.PullRequest {
	pullRequest.BaseRef = ref
	return pullRequest
}

func withHeadSHA(pullRequest review.PullRequest, sha string) review.PullRequest {
	pullRequest.HeadSHA = sha
	return pullRequest
}

func testDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(".", ".workspace-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove test directory: %v", err)
		}
	})
	return directory
}

func assertCommand(t *testing.T, commands []recordedCommand, expected []string) recordedCommand {
	t.Helper()
	for _, command := range commands {
		if slices.Equal(command.args, expected) {
			return command
		}
	}
	t.Fatalf("command not found: %q; commands: %+v", expected, commands)
	return recordedCommand{}
}

func countCommandsWithArg(commands []recordedCommand, name, arg string) int {
	count := 0
	for _, command := range commands {
		if len(command.args) > 0 && command.args[0] == name && slices.Contains(command.args, arg) {
			count++
		}
	}
	return count
}
