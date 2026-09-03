# rob-reviewer

`rob-reviewer` is a focused pull request review framework built with the [GitHub Copilot SDK for Go](https://github.com/github/copilot-sdk/tree/main/go). Its first production configuration is prepared to poll recently changed [`microsoft/vscode`](https://github.com/microsoft/vscode) pull requests and look for concrete, high-confidence performance regressions.

The framework runs one Copilot session per pull request. All enabled review-focus skills share that session, checkout, diff, and repository context. A custom reviewer agent performs a distinct pass for every focus and submits findings through typed host tools instead of returning prose that must be parsed.

## Behavior

- A GitHub Actions workflow polls every five minutes when the `ROB_REVIEWER_ENABLED` repository variable is `true`. Checked-in configuration also requires `automation.enabled: true`.
- The first enabled poll records every current open team PR and its current head without reviewing that backlog.
- Only PRs whose GitHub `author_association` is `MEMBER` or `OWNER` are eligible. Outside collaborators, contributors, bots, and other non-team authors are skipped.
- The poller detects newly opened PRs, drafts becoming ready, reopened PRs, and new head SHAs on existing PRs by scanning recently updated PRs.
- Every new head waits through a five-minute quiet period. Another push resets the clock, so rapid update bursts produce one review of the stable head.
- The same head is reviewed at most once. State is keyed by PR number and exact head SHA rather than only by PR number.
- `publication.mode: approval` writes JSON and Markdown reports for human selection. `publication.mode: automatic` publishes every validated finding after the report is durable. The checked-in mode is `automatic`.
- Automatic publication requires the PR to remain open, non-draft, unsuppressed, team-authored, and on the reviewed base/head at the final GitHub refresh. Explicitly approved saved reports may still be published to a closed or merged PR when the reviewed head matches.
- Per-run and per-UTC-day review caps bound model usage. A pending head older than the configured maximum age is skipped rather than producing a surprising late review.
- Adding the configured `performance-reviewer:skip` label suppresses that head.
- Every finding has a stable `PERF-...` ID for local reports and explicit publication. Automatic mode posts all host-validated findings.
- Publication posts a non-blocking `COMMENT` review with inline comments.
- Published summaries and inline comments begin with **Experimental performance review bot**.
- Findings must be performance-only, high confidence, anchored to an added or deleted line, and explicitly prove through before/after evidence that the PR introduced or materially amplified the performance mechanism. Pre-existing non-critical optimization opportunities are rejected.
- State is committed to a dedicated `reviewer-state` branch, including reviewed and pending heads plus daily review/publication counters.

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
- Model-authored review text is sanitized before publication: mentions, HTML, images, controls, bidi formatting, and active Markdown are neutralized. Well-formed inline code spans are retained.

The checkout layer keeps a persistent blobless bare cache under the user cache directory and creates a disposable shared checkout for each PR. Git history and objects are therefore amortized across PRs without sharing writable worktrees. The Actions workflow caches this directory between runs. Git stdout/stderr is bounded, and a unified diff over 32 MiB or 200,000 lines fails explicitly instead of exhausting runner memory.

## Repository configuration

[`reviewer.yaml`](reviewer.yaml) controls the target, model, confidence threshold, finding cap, automation gates, polling limits, publication mode, state location, and enabled focuses:

```yaml
version: 1
target:
  owner: microsoft
  repo: vscode
poll:
  maxPerRun: 5
  maxPerDay: 25
  quietPeriodMinutes: 5
  scanWindowHours: 168
  maxPendingAgeHours: 24
automation:
  enabled: true
  skipLabels:
    - performance-reviewer:skip
learning:
  enabled: true
  maxCasesPerRun: 1
publication:
  mode: automatic
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

The first enabled scheduled poll creates `reviewer-state` from the default branch when needed, upgrades any version-1 high-water state, snapshots current open heads, and exits. Branch protection must allow the workflow token to update that branch.

## Run history

Every poll invocation writes `run.json` and `run.md` into the workflow output directory. The run record includes:

- GitHub run ID, attempt, trigger, workflow, repository, and reviewer commit;
- start/completion timestamps and success or failure;
- bootstrap, reviewed, published, deferred, and skipped PR numbers;
- each analyzed PR's title, URL, base/head SHAs, finding count, and report filenames;
- the exact sanitized public comment bodies for findings freshly published by that run;
- a surfaced error and any partial completed work when polling fails.

Manually dispatched one-PR reviews use the same manifest and archive format.

The workflow first uploads the output as an Actions artifact. It then checks out the `reviewer-state` branch and commits the complete output directory under:

```text
.rob-reviewer/runs/YYYY/MM/DD/<github-run-id>-<attempt>/
```

Each workflow run therefore leaves an immutable, Git-browsable record alongside the mutable state file without adding generated reports to `main`. A resumed pending GitHub review is recorded in the run's `published` PR list; its original rendered comments remain in the earlier run record that created the pending review.

## Learning from shipped performance fixes

The performance skill also looks for PRs that remove a concrete, evidenced performance regression. This signal is separate from normal review findings and never creates a comment on the fixing PR.

For a high-confidence fixing PR, the reviewer:

1. identifies the changed line that removes or bounds the old mechanism;
2. uses the bounded `blame_base_line` tool against the PR's base revision;
3. records the blamed introducing commit and one predefined mechanism family;
4. asks GitHub which PR introduced that commit;
5. accepts only an earlier merged `MEMBER`/`OWNER` PR;
6. replays that introducing PR with the current performance skill, without publishing;
7. marks the case `detected` when the replay reports the same performance category on the implicated path, otherwise `instruction-gap`.

At most one introducer replay runs per workflow by default. The fixing PR, introducing PR/commit, mechanism, symptom, evidence, replay result, and coverage status are written to `regression-cases.json`. The workflow merges resolved pairs into the separate ledger:

```text
.rob-reviewer/regression-cases.json
```

The introducing PR's full replay report is stored alongside the fixing PR's run archive. Each global case records the GitHub run/attempt and archive path containing that evidence. This ledger is intentionally shaped for future benchmark generation, but no benchmark runner is enabled yet.

When replay indicates an instruction gap, the run's `learning-proposals.json` contains only the case ID and a constrained mechanism-family enum.

A separate `Apply reviewer learnings` workflow is triggered after the reviewer workflow completes. It accepts only runs produced from this repository's default branch, executes the current trusted `main` implementation rather than producer-controlled branch code, downloads that exact run's immutable Actions artifact, verifies the matching case archive exists on `reviewer-state`, and checks that every proposal matches a current-run case with the same family and `instruction-gap` result. This includes a run that completed learning but failed later while saving mutable poll state; runs without an artifact are a no-op, and an artifact without durable archived evidence is rejected.

The model, PR text, and mutable state ledger cannot authorize arbitrary instructions. Each mechanism family maps to a predefined, host-owned guidance paragraph; arbitrary mechanism/evidence prose remains only in the JSON case ledger. Updates are idempotent, modify only `learned-regressions.md`, use the GitHub Contents API with optimistic SHA checks, and carry the Copilot commit disclosure. The separate workflow can be rerun against the same immutable artifact if an update fails transiently.

## Automatic operation

The checked-in configuration enables automatic analysis and publication. The two Actions secrets must be configured and the `ROB_REVIEWER_ENABLED` repository variable must be `true` for scheduled runs.

The first enabled run bootstraps state and reviews no existing non-draft backlog. Later runs analyze new stable heads, write reports, and publish host-validated findings automatically.

To return to analysis-only operation, change `publication.mode` to `approval`.

Either switch is a kill switch:

- set `ROB_REVIEWER_ENABLED` to anything other than `true` to skip scheduled jobs immediately without a code change;
- set `automation.enabled: false` to make the poll command a no-op even if a workflow is dispatched.

Manual one-PR `review` and `publish-report` commands remain available while automation is disabled.

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

Publication does not rerun the model or require a Copilot token. It strictly parses the saved report, verifies that every finding ID still matches the complete finding content and the original PR number/base/head identity, selects only the approved IDs, revalidates the publication target, and posts one experimental non-blocking review. Finding IDs, confidence, raw evidence, and model statistics remain local. Each public inline comment includes severity and gives the local report's level of detail through a clear description of the changed behavior and its concrete impact, followed by the suggested fix. The publisher first creates a pending GitHub review as a concurrency claim and then submits that exact review. A retry resumes only a pending review with the same approved finding set.

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

The manual workflow supports the same modes: leave `pr_number` empty to invoke the configured poller or provide a number for one dry review. One-PR manual review never publishes. Polling follows `automation.enabled` and `publication.mode`. Manual review also enforces the team-author policy and refuses PRs whose author association is not `MEMBER` or `OWNER`.

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

Tests cover safe configuration defaults, state migration, open-head bootstrap, new heads, quiet-period resets, drafts becoming ready, reopened PRs, failed-head retries, daily caps, stale-head expiration, suppression labels, focus loading, safe checkout behavior, diff parsing and changed-line anchors, path containment, bounded tools, finding validation/ranking, stale-head rejection, publication idempotency, and review formatting. Synthetic fixtures model known performance failure mechanisms without copying VS Code source.

Use a manual dry-run workflow for the real SDK smoke test. Publishing happens separately from an approved saved report.

## Failure and retry behavior

- Processing stops at the first failed PR. Polling requires `--output-dir`, and both the authoritative JSON report and its Markdown rendering are atomically replaced inside the per-PR polling callback. A head is marked reviewed only after report persistence and, in automatic mode, successful publication.
- Reports completed before a later PR fails remain in the output directory and are uploaded by the workflow's `always()` artifact step. A later poll retries the failed PR.
- Run manifests are written from partial results before the poll command returns an error, so the artifact and state-branch archive show work completed before the failure.
- Pending heads are refreshed directly even after they leave the updated-PR scan window, so transient model/GitHub failures retry. Heads older than `maxPendingAgeHours` are deliberately skipped.
- If state persistence fails after a public review succeeds, the retry sees the authenticated PR/head marker, avoids duplicate comments, and then records the head as complete.
- If GitHub created a pending bot review but submission failed, the next automatic run resumes that review without rerunning the model. A newer head deletes the bot's stale pending review before proceeding; adding a suppression label deletes the current pending review instead of leaving it to block future heads.
- Publication has a separate lifecycle from polling state. Submitted reviews contain a hidden PR/head marker, while pending reviews bind to a digest of the approved finding set without exposing finding IDs. Only markers authored by the authenticated review identity count.
- Creating a pending review claims publication for that GitHub identity and PR. If submission fails, retrying the same approved IDs resumes that pending review; different IDs are rejected. Once submitted, later publication attempts for that PR/head are no-ops.
- Clean dry-run reports are persisted and advance polling state without creating any public marker or comment.
- UTC daily counters reset on date change. The per-day cap is persisted with the state branch, so separate workflow runs share one budget.
- Authentication, unavailable models, SDK startup, invalid tool submissions, incomplete focus passes, checkout errors, stale heads, and GitHub API errors fail explicitly; none are converted into clean reviews.
