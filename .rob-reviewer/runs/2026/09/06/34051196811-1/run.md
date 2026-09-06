# Reviewer run 34051196811

- Status: **succeeded**
- Started: `2026-09-06T18:17:29Z`
- Completed: `2026-09-06T18:26:13Z`
- Reviewer commit: `add9ed06110e74a6f47e8d38c1cb1acbbe1e60ab`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 347
- Skipped: 406
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#333606 — Recognize eagerness option in PatchBased02Unified](https://github.com/microsoft/vscode/pull/333606)

- Head: `6f8d0ee7dffec94091f1ee531b8bff35608fefc3`
- Findings: 0
- Published: false
- Reports: [pr-333606-6f8d0ee7dffe.json](pr-333606-6f8d0ee7dffe.json), [pr-333606-6f8d0ee7dffe.md](pr-333606-6f8d0ee7dffe.md)

### [#334521 — automations: refactor: make provider session templates canonical](https://github.com/microsoft/vscode/pull/334521)

- Head: `2dec72b1c8c47606885910917b7457e5fa08858e`
- Findings: 1
- Published: true
- Reports: [pr-334521-2dec72b1c8c4.json](pr-334521-2dec72b1c8c4.json), [pr-334521-2dec72b1c8c4.md](pr-334521-2dec72b1c8c4.md)

#### Published comment at `src/vs/sessions/contrib/automations/browser/automationDialog.ts:365`

**[Experimental performance review bot]**

**Severity: medium**

Retargeting now synchronously captures the previous provider draft before creating the requested draft; stale retarget iterations still pay that capture and leave the old draft installed, causing the next iteration to capture it again.

When an Agent Host/provider configuration request is slow or disconnected, quickly switching folder/provider/session type leaves the session controls unavailable for roughly two seconds for a common A→B→C correction, and further switches can add another timeout while also spawning additional never-settling capture promises.

**Suggested fix:** Coalesce capture by applied session: start at most one capture attempt for the old draft, share its result/deadline across queued target updates, and after it settles or times out create only the latest requested target. Also cancel or otherwise release the underlying provider capture when abandoning the draft.

(Written by Copilot)
