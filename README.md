# rob-reviewer

`rob-reviewer` is a focused pull request review framework built with the [GitHub Copilot SDK for Go](https://github.com/github/copilot-sdk/tree/main/go). Its first production configuration polls newly opened [`microsoft/vscode`](https://github.com/microsoft/vscode) pull requests and looks for concrete, high-confidence performance regressions.

The framework runs one Copilot session per pull request. All enabled review-focus skills share that session, checkout, diff, and repository context. A custom reviewer agent performs a distinct pass for every focus and submits findings through typed host tools instead of returning prose that must be parsed.

## Behavior

- A GitHub Actions workflow polls every ten minutes.
- The first poll records the current highest PR number and does not review the existing backlog.
- Only PRs whose GitHub `author_association` is `MEMBER` or `OWNER` are eligible. Outside collaborators, contributors, bots, and other non-team authors are skipped.
- Newly opened team-authored draft PRs are deferred until they become ready for review. Non-team drafts are skipped rather than persisted.
- Each eligible PR is reviewed once. New pushes to an already handled PR are not reviewed in version 1.
- A clean review is silent.
- A review with findings posts one non-blocking `COMMENT` review with at most ten inline comments.
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

Run one PR locally without publishing:

```bash
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer review --pr 123456
```

The dry-run result is written as JSON. Add `--publish` only when a public review is intended.

Use `--format markdown` for a readable local report. Both JSON and Markdown output include review statistics at the end: configured and actual model, configured and actual reasoning effort, model API endpoint, wall-clock time, model-call count, input/output/reasoning/cache token usage, aggregate model API time, tool-call count, Copilot nano-AI units, and model billing multipliers. The SDK does not expose a reliable USD conversion, so reports state that dollar cost is unavailable.

Scheduled reviews emit the same fields as structured workflow logs even when a clean review remains silent on GitHub. Reviews with findings include a collapsed statistics footer in the GitHub review summary.

Replay a closed or merged team-authored PR for regression testing:

```bash
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer review --pr 123456 --historical
```

Historical mode bypasses existing-review marker lookup and open-state publication checks so the original PR diff is analyzed again. It can never be combined with `--publish`.

Run the production poller:

```bash
GITHUB_REPOSITORY=owner/rob-reviewer \
GITHUB_TOKEN=... \
REVIEW_GITHUB_TOKEN=... \
COPILOT_GITHUB_TOKEN=... \
go run ./cmd/rob-reviewer poll
```

The manual workflow supports the same modes: leave `pr_number` empty to poll, provide a number for a dry run, or explicitly enable `publish`. Manual review also enforces the team-author policy and refuses PRs whose author association is not `MEMBER` or `OWNER`.

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

Use a manual dry-run workflow for the real SDK smoke test. Publishing is a separate explicit input.

## Failure and retry behavior

- Processing stops at the first failed PR. The high-water mark advances only through successful or intentionally skipped entries.
- A later poll retries the failed PR.
- Posted reviews contain a hidden PR/head marker. Only markers authored by the authenticated review identity count. If publication succeeded but state persistence failed, the retry sees the marker and does not duplicate comments.
- Clean reviews have no public marker by design. A rare state-write failure can repeat their analysis, but it cannot create public noise.
- Authentication, unavailable models, SDK startup, invalid tool submissions, incomplete focus passes, checkout errors, stale heads, and GitHub API errors fail explicitly; none are converted into clean reviews.
