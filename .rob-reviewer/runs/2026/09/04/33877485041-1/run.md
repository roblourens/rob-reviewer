# Reviewer run 33877485041

- Status: **succeeded**
- Started: `2026-09-04T13:20:53Z`
- Completed: `2026-09-04T13:27:30Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 340
- Skipped: 384
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332656 — chat: add telemetry for collapsible content toggles](https://github.com/microsoft/vscode/pull/332656)

- Head: `6f012acc1f7ecdb78109a7984b07c9c00a4413a0`
- Findings: 0
- Published: false
- Reports: [pr-332656-6f012acc1f7e.json](pr-332656-6f012acc1f7e.json), [pr-332656-6f012acc1f7e.md](pr-332656-6f012acc1f7e.md)

### [#334022 — Center full-width characters in two monospace cells](https://github.com/microsoft/vscode/pull/334022)

- Head: `4c0552f4f08c492b3ca7e817d1ed9870b78f55e3`
- Findings: 1
- Published: true
- Reports: [pr-334022-4c0552f4f08c.json](pr-334022-4c0552f4f08c.json), [pr-334022-4c0552f4f08c.md](pr-334022-4c0552f4f08c.md)

#### Published comment at `src/vs/editor/common/viewModel/monospaceLineBreaksComputer.ts:46`

**[Experimental performance review bot]**

**Severity: medium**

With forceFullwidthCharacterWidth enabled, the added condition rejects prior projection data unconditionally and sends every wrapped line through createLineBreaks, which scans the complete line and allocates fresh break-offset arrays and a new ModelLineProjectionData.

In a large CJK/Markdown document with viewport word wrap and this new option enabled, dragging a panel or resizing the editor synchronously reconstructs all model lines on every wrapping-column update, causing avoidable UI stalls and GC pressure.

**Suggested fix:** Retain the previous-data branch and call createLineBreaksFromPreviousLineBreaks with `forceFullwidthCharacterWidth ? 2 : columnsForFullWidthChar`; add a resize/previousLineBreakData test with the option enabled to preserve the exact two-column semantics.

(Written by Copilot)
