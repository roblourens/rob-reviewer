# Reviewer run 33957353041

- Status: **succeeded**
- Started: `2026-09-05T09:13:32Z`
- Completed: `2026-09-05T09:24:43Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 3
- Published: 2
- Deferred: 349
- Skipped: 389
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332071 — chat: support selective customization lockdown](https://github.com/microsoft/vscode/pull/332071)

- Head: `6e94e7f91cd5eb50907081287fd8d4cbb7d35db6`
- Findings: 0
- Published: false
- Reports: [pr-332071-6e94e7f91cd5.json](pr-332071-6e94e7f91cd5.json), [pr-332071-6e94e7f91cd5.md](pr-332071-6e94e7f91cd5.md)

### [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Head: `37e5ee7b1a9747057e9f3d6b4797ce90f27ba6e9`
- Findings: 2
- Published: true
- Reports: [pr-333196-37e5ee7b1a97.json](pr-333196-37e5ee7b1a97.json), [pr-333196-37e5ee7b1a97.md](pr-333196-37e5ee7b1a97.md)

#### Published comment at `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

The added fire-and-forget call invokes `reportCustomizationMigrationTelemetry` after every `_openChat`, including opens of side chats and subagent chats in a session that is already active.

Opening or switching among chats in an already-open Agents session calls `_openChat` each time, so normal tab navigation repeatedly launches the complete customization assessment even though the session lifecycle has not changed.

**Suggested fix:** Move this report to the actual session-entry transition (or guard/deduplicate it by session resource for the current open lifecycle) and leave chat-only navigation free of migration recomputation.

(Written by Copilot)

#### Published comment at `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

The new loop fire-and-forgets one `_reportCustomizationMigrationTelemetry` call for every already-resolved restored session without a limiter, idle deferral, or grouping by equivalent assessment scope.

Reloading an Agents window with several persisted visible sessions immediately launches a full customization assessment for every restored session just as the workbench is restoring. Different workspace roots prevent MCP-scope coalescing, so a realistic multi-session grid fans out concurrent work.

**Suggested fix:** Queue these reports with bounded concurrency or defer them to idle, and coalesce assessments that share the same session type/workspace scope while preserving the required per-session telemetry semantics.

(Written by Copilot)

### [#334341 — Add opt-in auto-archive inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341)

- Head: `7e6d14157062b843b8a4ca0efc58f6b2157f306b`
- Findings: 1
- Published: true
- Reports: [pr-334341-7e6d14157062.json](pr-334341-7e6d14157062.json), [pr-334341-7e6d14157062.md](pr-334341-7e6d14157062.md)

#### Published comment at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:144`

**[Experimental performance review bot]**

**Severity: medium**

The new hourly lifecycle restores every due deletion candidate before checking whether its retained worktree makes automatic deletion impossible; rejected candidates remain eligible and repeat the sequence next hour.

Automatically archived sessions whose worktree cleanup was skipped or failed repeatedly consume GitHub API budget and provider/database I/O every hour despite being unable to make deletion progress.

**Suggested fix:** For delete candidates, run a cheap `canDeleteSession` preflight before the GitHub refresh and restore, while retaining the current in-disposal recheck for race safety.

(Written by Copilot)
