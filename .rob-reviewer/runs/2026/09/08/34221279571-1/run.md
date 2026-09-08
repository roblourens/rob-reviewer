# Reviewer run 34221279571

- Status: **succeeded**
- Started: `2026-09-08T11:33:54Z`
- Completed: `2026-09-08T11:50:47Z`
- Reviewer commit: `add9ed06110e74a6f47e8d38c1cb1acbbe1e60ab`
- Bootstrapped: 0
- Reviewed: 4
- Published: 1
- Deferred: 347
- Skipped: 399
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332071 — chat: support selective customization lockdown](https://github.com/microsoft/vscode/pull/332071)

- Head: `676d01c2252dab6d85ee418070eb1ba59f880761`
- Findings: 0
- Published: false
- Reports: [pr-332071-676d01c2252d.json](pr-332071-676d01c2252d.json), [pr-332071-676d01c2252d.md](pr-332071-676d01c2252d.md)

### [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Head: `cd4d9a1f199af5462d0e97190012a87f28eca618`
- Findings: 1
- Published: true
- Reports: [pr-333196-cd4d9a1f199a.json](pr-333196-cd4d9a1f199a.json), [pr-333196-cd4d9a1f199a.md](pr-333196-cd4d9a1f199a.md)

#### Published comment at `src/vs/sessions/services/sessions/browser/sessionsService.ts:939`

**[Experimental performance review bot]**

**Severity: medium**

Startup restore now queues a full customization migration assessment for every restored session. Although each enqueue receives the restore cancellation token, queued and running factories discard it once admitted by the limiter.

If the user opens a session while startup is restoring several persisted sessions—especially while agent/customization state is cold—all stale restore assessments continue in the background. They consume filesystem/extension-provider I/O and MCP assessment CPU after the restore was abandoned, contending with the newly opened session and prolonging post-navigation activity.

**Suggested fix:** Recheck the restore token inside the limiter factory and pass it to `reportCustomizationMigrationTelemetry`; make the MCP assessment wait cancellation-aware as well so already-running work releases its scope promptly. This preserves the concurrency limit and telemetry for restores that remain active while dropping only superseded work.

(Written by Copilot)

### [#334774 — eslint: fix bracket notation in workbench services](https://github.com/microsoft/vscode/pull/334774)

- Head: `6eff68f00e819fff4bbb31e5b1639b1cee63dcaf`
- Findings: 0
- Published: false
- Reports: [pr-334774-6eff68f00e81.json](pr-334774-6eff68f00e81.json), [pr-334774-6eff68f00e81.md](pr-334774-6eff68f00e81.md)

### [#334833 — ci: Reduce test runner startup overhead](https://github.com/microsoft/vscode/pull/334833)

- Head: `7bc0c5d624cb0883fb813373a23aa7fc41544184`
- Findings: 0
- Published: false
- Reports: [pr-334833-7bc0c5d624cb.json](pr-334833-7bc0c5d624cb.json), [pr-334833-7bc0c5d624cb.md](pr-334833-7bc0c5d624cb.md)
