# Published comments: 1

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196) at `src/vs/sessions/services/sessions/browser/sessionsService.ts:939`

**[Experimental performance review bot]**

**Severity: medium**

Startup restore now queues a full customization migration assessment for every restored session. Although each enqueue receives the restore cancellation token, queued and running factories discard it once admitted by the limiter.

If the user opens a session while startup is restoring several persisted sessions—especially while agent/customization state is cold—all stale restore assessments continue in the background. They consume filesystem/extension-provider I/O and MCP assessment CPU after the restore was abandoned, contending with the newly opened session and prolonging post-navigation activity.

**Suggested fix:** Recheck the restore token inside the limiter factory and pass it to `reportCustomizationMigrationTelemetry`; make the MCP assessment wait cancellation-aware as well so already-running work releases its scope promptly. This preserves the concurrency limit and telemetry for restores that remain active while dropping only superseded work.

(Written by Copilot)

