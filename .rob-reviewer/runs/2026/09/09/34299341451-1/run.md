# Reviewer run 34299341451

- Status: **succeeded**
- Started: `2026-09-09T01:29:50Z`
- Completed: `2026-09-09T01:40:11Z`
- Reviewer commit: `22d2f3ec72332773d6fe6490753140d4f3d9418c`
- Bootstrapped: 0
- Reviewed: 3
- Published: 1
- Deferred: 337
- Skipped: 385
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#335111 — Remove featured sections from the customizations modal](https://github.com/microsoft/vscode/pull/335111)

- Head: `4dc6960dc8e50aa3ad4a98ee052eb068478b25ef`
- Findings: 0
- Published: false
- Reports: [pr-335111-4dc6960dc8e5.json](pr-335111-4dc6960dc8e5.json), [pr-335111-4dc6960dc8e5.md](pr-335111-4dc6960dc8e5.md)

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `b095443a4fb1395b984619437dcf2b371d114e8d`
- Findings: 1
- Published: true
- Reports: [pr-332410-b095443a4fb1.json](pr-332410-b095443a4fb1.json), [pr-332410-b095443a4fb1.md](pr-332410-b095443a4fb1.md)

#### Published comment at `src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:5342`

**[Experimental performance review bot]**

**Severity: medium**

Tool completion callbacks are now tracked in one set for the lifetime of the session wrapper, and every normal idle waits for all entries in that set, regardless of which turn created them.

After aborting an edit and immediately sending a replacement prompt, the replacement turn can appear stuck at completion until unrelated cleanup from the cancelled turn finishes, increasing end-of-turn latency in proportion to the slowest outstanding prior edit.

**Suggested fix:** Track completion promises by their captured turn and await only the current turn's set. On abort, detach that turn's entries from the idle barrier while still allowing their cleanup/persistence promises to settle.

(Written by Copilot)

### [#335173 — nes: fix: encode paths in inline suggestion requests](https://github.com/microsoft/vscode/pull/335173)

- Head: `25a5a5eb7703832e3001b55accce505555b58437`
- Findings: 0
- Published: false
- Reports: [pr-335173-25a5a5eb7703.json](pr-335173-25a5a5eb7703.json), [pr-335173-25a5a5eb7703.md](pr-335173-25a5a5eb7703.md)
