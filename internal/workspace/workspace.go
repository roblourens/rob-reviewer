package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/roblourens/rob-reviewer/internal/review"
)

const (
	commandStdoutLimit = int64(1 << 20)
	commandStderrLimit = int64(1 << 20)
	diffStdoutLimit    = int64(32 << 20)
)

var ErrOutputLimitExceeded = errors.New("git output limit exceeded")

type Workspace struct {
	Root      string
	MergeBase string
	HeadSHA   string
	Diff      []byte

	cleanupRoot string
	closeOnce   sync.Once
	closeErr    error
}

func (w *Workspace) DiffString() string {
	return string(w.Diff)
}

func (w *Workspace) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = os.RemoveAll(w.cleanupRoot)
	})
	return w.closeErr
}

type CommandError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("git %s: %v", quoteArgs(e.Args), e.Err)
	}
	return fmt.Sprintf("git %s: %v: %s", quoteArgs(e.Args), e.Err, e.Stderr)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

type OutputLimitError struct {
	Args   []string
	Stream string
	Limit  int64
}

func (e *OutputLimitError) Error() string {
	return fmt.Sprintf("git %s: %s exceeded %d-byte limit", quoteArgs(e.Args), e.Stream, e.Limit)
}

func (e *OutputLimitError) Unwrap() error {
	return ErrOutputLimitExceeded
}

func New(ctx context.Context, owner, repo string, pullRequest review.PullRequest) (*Workspace, error) {
	userCacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate user cache directory: %w", err)
	}
	cacheRoot := filepath.Join(userCacheRoot, "rob-reviewer")
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create rob-reviewer cache root: %w", err)
	}
	return newWithRunner(ctx, owner, repo, pullRequest, cacheRoot, execGitRunner{})
}

func Create(ctx context.Context, owner, repo string, pullRequest review.PullRequest) (*Workspace, error) {
	return New(ctx, owner, repo, pullRequest)
}

type commandLimits struct {
	stdout int64
	stderr int64
}

var defaultCommandLimits = commandLimits{
	stdout: commandStdoutLimit,
	stderr: commandStderrLimit,
}

type gitRunner interface {
	Run(context.Context, string, commandLimits, ...string) ([]byte, error)
}

type execGitRunner struct{}

func (execGitRunner) Run(
	ctx context.Context,
	directory string,
	limits commandLimits,
	args ...string,
) ([]byte, error) {
	baseArgs := []string{
		"--no-pager",
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "fetch.recurseSubmodules=false",
		"-c", "submodule.recurse=false",
		"-c", "diff.external=",
	}
	commandArgs := append(baseArgs, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Dir = directory
	command.Env = gitEnvironment()

	stdout := newLimitedBuffer(limits.stdout)
	stderr := newLimitedBuffer(limits.stderr)
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	if stdout.overflow {
		return nil, &OutputLimitError{
			Args:   append([]string(nil), commandArgs...),
			Stream: "stdout",
			Limit:  limits.stdout,
		}
	}
	if stderr.overflow {
		return nil, &OutputLimitError{
			Args:   append([]string(nil), commandArgs...),
			Stream: "stderr",
			Limit:  limits.stderr,
		}
	}
	if runErr != nil {
		return nil, &CommandError{
			Args:   append([]string(nil), commandArgs...),
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    runErr,
		}
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int64
	overflow bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - int64(buffer.buffer.Len())
	if remaining <= 0 {
		buffer.overflow = true
		return 0, ErrOutputLimitExceeded
	}
	if int64(len(data)) > remaining {
		written, _ := buffer.buffer.Write(data[:int(remaining)])
		buffer.overflow = true
		return written, ErrOutputLimitExceeded
	}
	return buffer.buffer.Write(data)
}

func (buffer *limitedBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}

func newWithRunner(
	ctx context.Context,
	owner string,
	repo string,
	pullRequest review.PullRequest,
	cacheRoot string,
	runner gitRunner,
) (_ *Workspace, returnErr error) {
	if err := validateRequest(owner, repo, pullRequest); err != nil {
		return nil, err
	}

	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	repositoryCache, err := ensureRepositoryCache(ctx, runner, cacheRoot, owner, repo, remoteURL)
	if err != nil {
		return nil, err
	}

	baseRef := cachedCommitRef(pullRequest.BaseSHA)
	baseSources := []string{pullRequest.BaseSHA, "refs/heads/" + pullRequest.BaseRef}
	if err := fetchCommit(
		ctx,
		runner,
		repositoryCache,
		remoteURL,
		baseRef,
		pullRequest.BaseSHA,
		baseSources,
	); err != nil {
		return nil, fmt.Errorf("fetch base commit %s: %w", pullRequest.BaseSHA, err)
	}
	headRef := cachedCommitRef(pullRequest.HeadSHA)
	headSources := []string{
		pullRequest.HeadSHA,
		"refs/pull/" + strconv.Itoa(pullRequest.Number) + "/head",
		"refs/heads/" + pullRequest.HeadRef,
	}
	if err := fetchCommit(
		ctx,
		runner,
		repositoryCache,
		remoteURL,
		headRef,
		pullRequest.HeadSHA,
		headSources,
	); err != nil {
		return nil, fmt.Errorf("fetch head commit %s: %w", pullRequest.HeadSHA, err)
	}

	workspaceParent := filepath.Join(cacheRoot, "workspaces")
	if err := os.MkdirAll(workspaceParent, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace parent: %w", err)
	}
	root, err := os.MkdirTemp(workspaceParent, "checkout-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary workspace: %w", err)
	}
	workspace := &Workspace{Root: root, cleanupRoot: root}
	defer func() {
		if returnErr != nil {
			if cleanupErr := workspace.Close(); cleanupErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("clean up workspace: %w", cleanupErr))
			}
		}
	}()

	if _, err := runner.Run(
		ctx,
		root,
		defaultCommandLimits,
		"clone",
		"--shared",
		"--no-checkout",
		"--no-tags",
		"--quiet",
		"--",
		repositoryCache,
		".",
	); err != nil {
		return nil, fmt.Errorf("create shared workspace clone: %w", err)
	}
	if err := configureWorkspaceRemote(ctx, runner, root, remoteURL); err != nil {
		return nil, err
	}

	mergeBaseOutput, err := runner.Run(
		ctx,
		root,
		defaultCommandLimits,
		"merge-base",
		pullRequest.BaseSHA,
		pullRequest.HeadSHA,
	)
	if err != nil {
		return nil, fmt.Errorf("compute merge base: %w", err)
	}
	mergeBase := strings.TrimSpace(string(mergeBaseOutput))
	if !validSHA(mergeBase) {
		return nil, fmt.Errorf("git merge-base returned invalid SHA %q", mergeBase)
	}

	if _, err := runner.Run(
		ctx,
		root,
		defaultCommandLimits,
		"checkout",
		"--quiet",
		"--detach",
		"--force",
		pullRequest.HeadSHA,
	); err != nil {
		return nil, fmt.Errorf("check out head commit: %w", err)
	}
	checkedOutHead, err := resolveRef(ctx, runner, root, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("verify checked out head: %w", err)
	}
	if !strings.EqualFold(checkedOutHead, pullRequest.HeadSHA) {
		return nil, fmt.Errorf("checked out head is %s, expected %s", checkedOutHead, pullRequest.HeadSHA)
	}

	diffOutput, err := runner.Run(
		ctx,
		root,
		commandLimits{stdout: diffStdoutLimit, stderr: commandStderrLimit},
		"diff",
		"--no-ext-diff",
		"--no-textconv",
		"--find-renames",
		"--unified=3",
		mergeBase,
		pullRequest.HeadSHA,
		"--",
	)
	if err != nil {
		return nil, fmt.Errorf("compute pull request diff: %w", err)
	}

	workspace.MergeBase = mergeBase
	workspace.HeadSHA = checkedOutHead
	workspace.Diff = append([]byte(nil), diffOutput...)
	return workspace, nil
}

func ensureRepositoryCache(
	ctx context.Context,
	runner gitRunner,
	cacheRoot string,
	owner string,
	repo string,
	remoteURL string,
) (string, error) {
	repositoryParent := filepath.Join(cacheRoot, "repositories", owner)
	if err := os.MkdirAll(repositoryParent, 0o700); err != nil {
		return "", fmt.Errorf("create repository cache parent: %w", err)
	}
	cachePath := filepath.Join(repositoryParent, repo+".git")
	if _, err := os.Lstat(cachePath); err == nil {
		if err := validateRepositoryCache(ctx, runner, cachePath); err != nil {
			return "", err
		}
		return cachePath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect repository cache: %w", err)
	}

	stagingPath, err := os.MkdirTemp(repositoryParent, "."+repo+"-init-*")
	if err != nil {
		return "", fmt.Errorf("create repository cache staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	cacheCommands := [][]string{
		{
			"clone",
			"--bare",
			"--filter=blob:none",
			"--no-tags",
			"--quiet",
			"--",
			remoteURL,
			".",
		},
		{"config", "remote.origin.promisor", "true"},
		{"config", "remote.origin.partialclonefilter", "blob:none"},
		{"config", "gc.auto", "0"},
	}
	for _, args := range cacheCommands {
		if _, err := runner.Run(ctx, stagingPath, defaultCommandLimits, args...); err != nil {
			return "", fmt.Errorf("initialize repository cache: %w", err)
		}
	}

	if err := os.Rename(stagingPath, cachePath); err != nil {
		if _, statErr := os.Lstat(cachePath); statErr != nil {
			return "", fmt.Errorf("publish repository cache: %w", err)
		}
		if err := validateRepositoryCache(ctx, runner, cachePath); err != nil {
			return "", err
		}
		return cachePath, nil
	}
	published = true
	return cachePath, nil
}

func validateRepositoryCache(
	ctx context.Context,
	runner gitRunner,
	cachePath string,
) error {
	info, err := os.Lstat(cachePath)
	if err != nil {
		return fmt.Errorf("inspect repository cache: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository cache %q is not a directory", cachePath)
	}
	output, err := runner.Run(
		ctx,
		cachePath,
		defaultCommandLimits,
		"rev-parse",
		"--is-bare-repository",
	)
	if err != nil {
		return fmt.Errorf("validate repository cache: %w", err)
	}
	if strings.TrimSpace(string(output)) != "true" {
		return fmt.Errorf("repository cache %q is not bare", cachePath)
	}
	return nil
}

func configureWorkspaceRemote(
	ctx context.Context,
	runner gitRunner,
	root string,
	remoteURL string,
) error {
	commands := [][]string{
		{"remote", "set-url", "origin", remoteURL},
		{"config", "remote.origin.promisor", "true"},
		{"config", "remote.origin.partialclonefilter", "blob:none"},
		{"config", "extensions.partialClone", "origin"},
	}
	for _, args := range commands {
		if _, err := runner.Run(ctx, root, defaultCommandLimits, args...); err != nil {
			return fmt.Errorf("configure workspace remote: %w", err)
		}
	}
	return nil
}

func fetchCommit(
	ctx context.Context,
	runner gitRunner,
	repositoryCache string,
	remoteURL string,
	destination string,
	expectedSHA string,
	sources []string,
) error {
	var fetchErrors []error
	for _, source := range sources {
		refspec := "+" + source + ":" + destination
		if _, err := runner.Run(
			ctx,
			repositoryCache,
			defaultCommandLimits,
			"fetch",
			"--quiet",
			"--force",
			"--filter=blob:none",
			"--no-tags",
			"--no-write-fetch-head",
			remoteURL,
			refspec,
		); err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("fetch %s: %w", source, err))
			if actualSHA, resolveErr := resolveRef(ctx, runner, repositoryCache, destination); resolveErr == nil &&
				strings.EqualFold(actualSHA, expectedSHA) {
				return nil
			}
			continue
		}
		actualSHA, err := resolveRef(ctx, runner, repositoryCache, destination)
		if err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("resolve fetched ref from %s: %w", source, err))
			continue
		}
		if !strings.EqualFold(actualSHA, expectedSHA) {
			fetchErrors = append(fetchErrors, fmt.Errorf(
				"source %s resolved to %s, expected %s",
				source,
				actualSHA,
				expectedSHA,
			))
			continue
		}
		return nil
	}
	return errors.Join(fetchErrors...)
}

func resolveRef(ctx context.Context, runner gitRunner, root, ref string) (string, error) {
	output, err := runner.Run(
		ctx,
		root,
		defaultCommandLimits,
		"rev-parse",
		"--verify",
		ref+"^{commit}",
	)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(output))
	if !validSHA(sha) {
		return "", fmt.Errorf("rev-parse returned invalid SHA %q", sha)
	}
	return sha, nil
}

func cachedCommitRef(sha string) string {
	return "refs/rob-reviewer/commits/" + strings.ToLower(sha)
}

func validateRequest(owner, repo string, pullRequest review.PullRequest) error {
	if !validOwner(owner) {
		return fmt.Errorf("invalid GitHub owner %q", owner)
	}
	if !validRepo(repo) {
		return fmt.Errorf("invalid GitHub repository %q", repo)
	}
	if pullRequest.Number < 1 {
		return errors.New("pull request number must be positive")
	}
	if !validSHA(pullRequest.BaseSHA) {
		return fmt.Errorf("invalid base SHA %q", pullRequest.BaseSHA)
	}
	if !validSHA(pullRequest.HeadSHA) {
		return fmt.Errorf("invalid head SHA %q", pullRequest.HeadSHA)
	}
	if !validBranch(pullRequest.BaseRef) {
		return fmt.Errorf("invalid base ref %q", pullRequest.BaseRef)
	}
	if !validBranch(pullRequest.HeadRef) {
		return fmt.Errorf("invalid head ref %q", pullRequest.HeadRef)
	}
	return nil
}

func validOwner(value string) bool {
	if value == "" || len(value) > 39 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if !isASCIILetterOrDigit(character) && character != '-' {
			return false
		}
	}
	return true
}

func validRepo(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if !isASCIILetterOrDigit(character) && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func validBranch(value string) bool {
	if value == "" ||
		value == "@" ||
		strings.HasPrefix(value, "-") ||
		strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") ||
		strings.HasSuffix(value, ".") ||
		strings.Contains(value, "..") ||
		strings.Contains(value, "@{") ||
		strings.Contains(value, "//") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	for _, character := range value {
		if unicode.IsControl(character) ||
			unicode.IsSpace(character) ||
			strings.ContainsRune(`~^:?*[\`, character) {
			return false
		}
	}
	return true
}

func validSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') &&
			!(character >= 'a' && character <= 'f') &&
			!(character >= 'A' && character <= 'F') {
			return false
		}
	}
	return true
}

func isASCIILetterOrDigit(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func gitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(name, "GIT_") {
			environment = append(environment, variable)
		}
	}
	return append(
		environment,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = strconv.Quote(arg)
	}
	return strings.Join(quoted, " ")
}
