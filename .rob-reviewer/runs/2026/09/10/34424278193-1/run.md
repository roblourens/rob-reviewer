# Reviewer run 34424278193

- Status: **succeeded**
- Started: `2026-09-10T01:09:52Z`
- Completed: `2026-09-10T01:22:38Z`
- Reviewer commit: `88002dfb88d8ae1a3ca3d405307ccd163af1eae3`
- Bootstrapped: 0
- Reviewed: 3
- Published: 1
- Deferred: 338
- Skipped: 36
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334972 — Implement experimental mode and permissions picker integration](https://github.com/microsoft/vscode/pull/334972)

- Head: `6276cb1e26404118ff5468e28b6668295cb5998b`
- Findings: 0
- Published: false
- Reports: [pr-334972-6276cb1e2640.json](pr-334972-6276cb1e2640.json), [pr-334972-6276cb1e2640.md](pr-334972-6276cb1e2640.md)

### [#335366 — chat: Show MCP server error details in customizations](https://github.com/microsoft/vscode/pull/335366)

- Head: `324e07d15719037092f9c28704c5ce3003fe143c`
- Findings: 2
- Published: true
- Reports: [pr-335366-324e07d15719.json](pr-335366-324e07d15719.json), [pr-335366-324e07d15719.md](pr-335366-324e07d15719.md)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:383`

**[Experimental performance review bot]**

**Severity: medium**

The renderer now assigns the complete raw error message to textContent and the accompanying CSS uses pre-wrap, overflow-wrap:anywhere, and visible overflow, so no display or measurement bound remains.

Opening Customizations after an HTTP MCP endpoint returns a large error body, or after an SDK reports verbose startup diagnostics, can freeze or severely jank the editor while that full payload is wrapped and measured.

**Suggested fix:** Cap the inline/ARIA preview by bytes or lines before DOM assignment and constrain row height, while preserving access to the complete diagnostic through Show Output or an explicit on-demand expansion that performs the large layout only when requested.

(Written by Copilot)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:1747`

**[Experimental performance review bot]**

**Severity: medium**

A new section autorun reads every cached row label on any shared customization/session change, marks the whole list for remeasurement, and schedules rerender.

With many installed MCP servers, startup, failure, recovery, or enablement updates can repeatedly delay interaction and rendering on the Customizations page.

**Suggested fix:** Leave label subscriptions with rendered accessibility rows. Use one shared status/session subscription to request layout, and invalidate only rows whose error text or visibility changed rather than eagerly evaluating all labels and rerendering the whole list.

(Written by Copilot)

### [#335369 — Suggest TypeScript 7 for users with no plugins enabled](https://github.com/microsoft/vscode/pull/335369)

- Head: `3c7ec43f21fa2efac85094c32837683113bf73d2`
- Findings: 0
- Published: false
- Reports: [pr-335369-3c7ec43f21fa.json](pr-335369-3c7ec43f21fa.json), [pr-335369-3c7ec43f21fa.md](pr-335369-3c7ec43f21fa.md)
