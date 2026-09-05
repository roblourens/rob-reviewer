# Reviewer run 33975248149

- Status: **succeeded**
- Started: `2026-09-05T15:35:54Z`
- Completed: `2026-09-05T15:52:44Z`
- Reviewer commit: `6e28f03a0606c936c6ac19020e0c7b05253277fa`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 347
- Skipped: 393
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `3392bf9d61d3b5c848eb09bfd8d7d7de5e404713`
- Findings: 1
- Published: true
- Reports: [pr-332410-3392bf9d61d3.json](pr-332410-3392bf9d61d3.json), [pr-332410-3392bf9d61d3.md](pr-332410-3392bf9d61d3.md)

#### Published comment at `src/vs/platform/agentHost/node/agentService.ts:5313`

**[Experimental performance review bot]**

**Severity: medium**

The passive toggle now awaits `_synchronizePassiveSessionMetadata` before updating surfaced state. That synchronization reads the local snapshot and central row, writes a local pending snapshot, upserts the central catalog, and writes a local acknowledgement, after which this method also marks the payload dirty.

Marking an unopened session read or archived can now remain pending while several SQLite operations complete, and any subsequent action from that client waits behind the same dispatch promise. This is especially visible while startup migration or reconciliation is using the central database sequencer.

**Suggested fix:** After the original metadata write, update/dispatch the surfaced state immediately and enqueue the catalog projection on the existing per-session sequencer/background-write tracker. Preserve deletion ordering by draining that tracked work, and invalidate/reconcile the list when the projection finishes.

(Written by Copilot)

### [#334695 — Fix Copilot Sessions Provider to Resolve Changes Summary Correctly](https://github.com/microsoft/vscode/pull/334695)

- Head: `943a50b434d7747e13026ce63170376613c3f387`
- Findings: 0
- Published: false
- Reports: [pr-334695-943a50b434d7.json](pr-334695-943a50b434d7.json), [pr-334695-943a50b434d7.md](pr-334695-943a50b434d7.md)
