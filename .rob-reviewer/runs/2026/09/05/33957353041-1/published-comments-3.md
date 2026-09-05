# Published comments: 3

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196) at `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

The added fire-and-forget call invokes `reportCustomizationMigrationTelemetry` after every `_openChat`, including opens of side chats and subagent chats in a session that is already active.

Opening or switching among chats in an already-open Agents session calls `_openChat` each time, so normal tab navigation repeatedly launches the complete customization assessment even though the session lifecycle has not changed.

**Suggested fix:** Move this report to the actual session-entry transition (or guard/deduplicate it by session resource for the current open lifecycle) and leave chat-only navigation free of migration recomputation.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196) at `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

The new loop fire-and-forgets one `_reportCustomizationMigrationTelemetry` call for every already-resolved restored session without a limiter, idle deferral, or grouping by equivalent assessment scope.

Reloading an Agents window with several persisted visible sessions immediately launches a full customization assessment for every restored session just as the workbench is restoring. Different workspace roots prevent MCP-scope coalescing, so a realistic multi-session grid fans out concurrent work.

**Suggested fix:** Queue these reports with bounded concurrency or defer them to idle, and coalesce assessments that share the same session type/workspace scope while preserving the required per-session telemetry semantics.

(Written by Copilot)

## [#334341 — Add opt-in auto-archive inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341) at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:144`

**[Experimental performance review bot]**

**Severity: medium**

The new hourly lifecycle restores every due deletion candidate before checking whether its retained worktree makes automatic deletion impossible; rejected candidates remain eligible and repeat the sequence next hour.

Automatically archived sessions whose worktree cleanup was skipped or failed repeatedly consume GitHub API budget and provider/database I/O every hour despite being unable to make deletion progress.

**Suggested fix:** For delete candidates, run a cheap `canDeleteSession` preflight before the GitHub refresh and restore, while retaining the current in-disposal recheck for race safety.

(Written by Copilot)

