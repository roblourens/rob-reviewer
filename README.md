# rob-reviewer

`rob-reviewer` is a focused pull request review framework built with the [GitHub Copilot SDK for Go](https://github.com/github/copilot-sdk/tree/main/go). Its first production configuration polls newly opened [`microsoft/vscode`](https://github.com/microsoft/vscode) pull requests and looks for concrete, high-confidence performance regressions.

The framework runs one Copilot session per pull request. All enabled review-focus skills share that session, checkout, diff, and repository context. A custom reviewer agent performs a distinct pass for every focus and submits findings through typed host tools instead of returning prose that must be parsed.

## Behavior

- A GitHub Actions workflow polls every ten minutes.
- The first poll records the current highest PR number and does not review the existing backlog.
- Only PRs whose GitHub `author_association` is `MEMBER` or `OWNER` are eligible. Outside collaborators, contributors, bots, and other non-team authors are skipped.
- Newly opened team-authored draft PRs are deferred until they become ready for review. Non-team drafts are skipped rather than persisted.
- Each eligible PR is reviewed once. New pushes to an already handled PR are not reviewed in version 1.
- Scheduled and manual analysis is dry-run only. It writes JSON and Markdown reports and never posts automatically.
- Every finding has a stable `PERF-...` ID. A human explicitly approves IDs from a saved JSON report before publication.
- One approved publication posts a non-blocking `COMMENT` review with only the selected inline comments.
- Published summaries and inline comments begin with **Experimental performance review bot**.
- Findings must be performance-only, high confidence, anchored to an added or deleted line, and explicitly prove through before/after evidence that the PR introduced or materially amplified the performance mechanism. Pre-existing non-critical optimization opportunities are rejected.
- State is committed to a dedicated `reviewer-state` branch, including silent clean results and deferred drafts.

Scheduled workflows are eventually consistent: GitHub may delay cron jobs. GitHub also disables scheduled workflows in an inactive public repository after 60 days, so keep the repository active or re-enable the workflow when needed.

## Security model

Pull requests are untrusted input.

- The checkout never runs PR code, hooks, submodules, package scripts, tests, or builds.
- Copilot runs in SDK `ModeEmpty`.
- Repository custom instructions, file hooks, host git operations, session memory, and ambient config discovery are disabled.
- The model receives only bounded host-owned tools for PR metadata, changed-file inventory, diff reads, file reads, repository search, focus documents, finding submission, and completion.
- Changed-file and diff tools are paginated. File reads can target later line ranges without loading an entire file.
- It receives no shell, network, write, arbitrary MCP, or repository-execution tool.
- Paths are resolved inside the checkout, symlink escapes are rejected, search and read output are capped, and inline anchors are validated against actual added/deleted diff lines.
- The head SHA is checked again after analysis. A changed head fails the run instead of advancing state or publishing a stale result.
- Model-authored review text is rendered as escaped plain Markdown text: mentions, HTML, images, control characters, and formatting delimiters cannot become active GitHub content.

The checkout layer keeps a persistent blobless bare cache under the user cache directory and creates a disposable shared checkout for each PR. Git history and objects are therefore amortized across PRs without sharing writable worktrees. The Actions workflow caches this directory between runs. Git stdout/stderr is bounded, and a unified diff over 32 MiB or 200,000 lines fails explicitly instead of exhausting runner memory.

## Repository configuration

[`reviewer.yaml`](reviewer.yaml) controls the target, model, confidence threshold, finding cap, poll batch size, state location, and enabled focuses:

```yaml
version: 1
target:
  owner: microsoft
  repo: vscode
poll:
  maxPerRun: 5
review:
  model: gpt-5.6-sol
  reasoningEffort: high
  minConfidence: 0.85
  maxFindings: 10
  focuses:
    - performance-review
state:
  branch: reviewer-state
  path: .rob-reviewer/state.json
```

`COPILOT_MODEL` overrides the configured model. The reviewer calls `ListModels` at startup and fails with the available model IDs if the configured model is unavailable.

## Credentials

Configure two Actions secrets:

- `COPILOT_GITHUB_TOKEN`: a GitHub user token accepted by the Copilot SDK for an account with Copilot access.
- `REVIEW_GITHUB_TOKEN`: a GitHub user credential that can read public PRs and submit reviews to `microsoft/vscode`. The repository-scoped Actions token cannot write to another repository. Confirm the selected credential's scope and organization policy before enabling publication.

The workflow-provided `GITHUB_TOKEN` is used only for the state branch in this repository and has `contents: write`. Keep the Copilot and review credentials separate even if one user owns both. Workflow actions are pinned to immutable commits, checkout credentials are not persisted, and the review/Copilot secrets are scoped only to the application steps.

The first scheduled poll creates `reviewer-state` from the default branch when needed, writes the bootstrap high-water mark, and exits. Branch protection must allow the workflow token to update that branch.

## Commands

Go 1.24 or later and `rg` (ripgrep) are required. The GitHub Actions workflows install ripgrep explicitly.

The Go SDK CLI bundler is pinned as a Go tool. Run it before building so the resulting application carries the matching Copilot CLI:

```bash
go mod download
(cd cmd/rob-reviewer && go tool bundler)
go build ./cmd/rob-reviewer
```

Run one PR locally:

```bash
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer review --pr 123456 --output-dir review-results
```

The command writes both JSON and Markdown. Read the Markdown report and note the `PERF-...` IDs you approve.

Use `--format markdown` for a readable local report. Both JSON and Markdown output include review statistics at the end: configured and actual model, configured and actual reasoning effort, model API endpoint, wall-clock time, model-call count, input/output/reasoning/cache token usage, aggregate model API time, tool-call count, Copilot nano-AI units, and model billing multipliers. The SDK does not expose a reliable USD conversion, so reports state that dollar cost is unavailable.

Scheduled reviews upload the same JSON and Markdown files as a workflow artifact. They also emit complete statistics in structured logs.

Publish only explicitly approved IDs from the saved JSON report:

```bash
REVIEW_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer publish-report \
  --report review-results/pr-123456-abcdef123456.json \
  --finding PERF-1234567890AB \
  --finding PERF-ABCDEF123456
```

Publication does not rerun the model or require a Copilot token. It strictly parses the saved report, verifies that every finding ID still matches the complete finding content and the original PR number/base/head identity, selects only the approved IDs, revalidates the publication target, and posts one experimental non-blocking review. Finding IDs, confidence, severity, raw evidence, and model statistics remain local. Each public inline comment gives the local report's level of detail through a clear description of the changed behavior and its concrete impact, followed by the suggested fix. The publisher first creates a pending GitHub review as a concurrency claim and then submits that exact review. A retry resumes only a pending review with the same approved finding set.

Replay a closed or merged team-authored PR for regression testing:

```bash
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer review --pr 123456 --historical
```

Historical mode bypasses existing-review marker lookup while analyzing the original PR diff. A saved historical report can be published after explicit approval even if the PR has closed or merged. Publication still requires the same team-authored PR and exact reviewed head SHA; for an open PR it also requires the exact base SHA and rejects drafts. A closed PR's base branch may advance after closure, so publication does not compare its current base SHA.

Run the production poller:

```bash
GITHUB_REPOSITORY=owner/rob-reviewer \
GITHUB_TOKEN=... \
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer poll --output-dir review-results
```

The manual workflow supports the same dry-run modes: leave `pr_number` empty to poll or provide a number for one review. It uploads reports as an artifact and never publishes. Manual review also enforces the team-author policy and refuses PRs whose author association is not `MEMBER` or `OWNER`.

## Adding a review focus

Create a directory under [`reviewers/`](reviewers/) with a `SKILL.md`:

```text
reviewers/
  my-focus/
    SKILL.md
    references/
      evidence.md
```

The skill must have YAML frontmatter with a unique `name`:

```markdown
---
name: my-focus-review
description: Find a narrow class of high-confidence bugs introduced by a diff.
---
```

Put stable reviewer behavior in `SKILL.md`. Put evolving examples, incident evidence, and longer domain references in supporting Markdown documents. The skill can direct the reviewer to load those documents with the bounded focus-document tool.

Add the skill name, not the directory name, to `review.focuses`. The framework validates every configured name, preloads all enabled skills into one custom agent, and requires the agent to complete every focus pass exactly once.

The starter [`performance-review` skill](reviewers/performance/SKILL.md) loads a generic [performance review guide](reviewers/performance/references/performance-review-guide.md) covering scaling, hot paths, rendering, memory, caching, I/O, concurrency, startup, scheduling, and evidence standards. [External research foundations](reviewers/performance/references/research-foundations.md) record the LLM-review and browser/Electron sources behind the workflow, while short [VS Code regression case notes](reviewers/performance/references/vscode-regressions.md) preserve project-specific provenance rather than serving as the review checklist.

## Testing

Normal CI is deterministic and does not require GitHub or Copilot credentials:

```bash
go test ./...
go vet ./...
(cd cmd/rob-reviewer && go tool bundler)
go build ./cmd/rob-reviewer
```

Tests cover configuration, focus loading, team-author eligibility, polling/bootstrap/deferred drafts, state persistence, safe checkout behavior, diff parsing and changed-line anchors, path containment, bounded tools, finding validation/ranking, stale-head rejection, idempotency, and review formatting. Synthetic fixtures model known performance failure mechanisms without copying VS Code source.

Use a manual dry-run workflow for the real SDK smoke test. Publishing happens separately from an approved saved report.

## Failure and retry behavior

- Processing stops at the first failed PR. Polling requires `--output-dir`, and both the authoritative JSON report and its Markdown rendering are atomically replaced inside the per-PR polling callback. The high-water mark advances only after those files are durable, or after an entry is intentionally skipped.
- Reports completed before a later PR fails remain in the output directory and are uploaded by the workflow's `always()` artifact step. A later poll retries the failed PR.
- Publication has a separate lifecycle from polling state. Submitted reviews contain a hidden PR/head marker, while pending reviews bind to a digest of the approved finding set without exposing finding IDs. Only markers authored by the authenticated review identity count.
- Creating a pending review claims publication for that GitHub identity and PR. If submission fails, retrying the same approved IDs resumes that pending review; different IDs are rejected. Once submitted, later publication attempts for that PR/head are no-ops.
- Clean dry-run reports are persisted and advance polling state without creating any public marker or comment.
- Authentication, unavailable models, SDK startup, invalid tool submissions, incomplete focus passes, checkout errors, stale heads, and GitHub API errors fail explicitly; none are converted into clean reviews.
