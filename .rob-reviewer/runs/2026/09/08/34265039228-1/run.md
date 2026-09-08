# Reviewer run 34265039228

- Status: **succeeded**
- Started: `2026-09-08T18:47:46Z`
- Completed: `2026-09-08T19:06:37Z`
- Reviewer commit: `22d2f3ec72332773d6fe6490753140d4f3d9418c`
- Bootstrapped: 0
- Reviewed: 5
- Published: 3
- Deferred: 352
- Skipped: 390
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Head: `88b5697549698ed3807c3f0f5c149cc0a680c923`
- Findings: 0
- Published: false
- Reports: [pr-333196-88b569754969.json](pr-333196-88b569754969.json), [pr-333196-88b569754969.md](pr-333196-88b569754969.md)

### [#334513 — feat: implementing word wrap indicators](https://github.com/microsoft/vscode/pull/334513)

- Head: `1bc3356bea0674bd4c6a65f6a703ddb4481afd8d`
- Findings: 1
- Published: true
- Reports: [pr-334513-1bc3356bea06.json](pr-334513-1bc3356bea06.json), [pr-334513-1bc3356bea06.md](pr-334513-1bc3356bea06.md)

#### Published comment at `src/vs/editor/browser/viewParts/wordWrapIndicator/wordWrapIndicator.ts:108`

**[Experimental performance review bot]**

**Severity: medium**

The new overlay calls ctx.viewportData.getViewLineRenderingData for every visible line on each invalidation even though it reads only continuesWithWrappedLine.

With wordWrapIndicator enabled, a tall wrapped editor containing many visible decorations (for example search matches, diagnostics, and inline decorations) repeats the viewport-decoration scan once per visible line whenever scrolling or decoration changes trigger rendering; hundreds of visible lines and decorations can therefore produce tens of thousands of checks on a frame and cause dropped frames.

**Suggested fix:** Read continuesWithWrappedLine from this.\_context.viewModel.getViewLineData(lineNumber) (or add an equally narrow accessor) so the pass remains O(V) and avoids allocating full rendering data.

(Written by Copilot)

### [#335048 — Preserve Git repository wrapper identity](https://github.com/microsoft/vscode/pull/335048)

- Head: `699cdc21d2d93bdb3aa21759ac26c4354a4649a9`
- Findings: 0
- Published: false
- Reports: [pr-335048-699cdc21d2d9.json](pr-335048-699cdc21d2d9.json), [pr-335048-699cdc21d2d9.md](pr-335048-699cdc21d2d9.md)

### [#335053 — agentHost: surface Codex skill validation errors](https://github.com/microsoft/vscode/pull/335053)

- Head: `3dc67c0c882fdd00f005130ec907fec2325bebe2`
- Findings: 1
- Published: true
- Reports: [pr-335053-3dc67c0c882f.json](pr-335053-3dc67c0c882f.json), [pr-335053-3dc67c0c882f.md](pr-335053-3dc67c0c882f.md)

#### Published comment at `src/vs/platform/agentHost/node/codex/codexCustomizations.ts:362`

**[Experimental performance review bot]**

**Severity: medium**

Invalid skill paths are now projected as E separate top-level containers, which the existing refresh publisher emits as E independent state updates.

When a workspace contains many invalid or legacy skills (for example, a collection whose SKILL.md files all omit the newly required description), every coalesced `skills/changed` refresh now publishes one `SessionCustomizationUpdated` action per invalid file. Those actions are processed synchronously and individually, so repairing or editing skills can cause noticeable Customizations UI churn and agent-host IPC/CPU spikes.

**Suggested fix:** Preserve the visible per-skill diagnostics but batch their state publication—for example, add a directory-customization batch replacement action—or group children that share the same diagnostic into one error container so a common schema failure does not create one top-level update per file.

(Written by Copilot)

### [#335070 — Add word wrap control to Agents editors](https://github.com/microsoft/vscode/pull/335070)

- Head: `841383d8071492f0ba7b7e04b123d0da02574eb1`
- Findings: 1
- Published: true
- Reports: [pr-335070-841383d80714.json](pr-335070-841383d80714.json), [pr-335070-841383d80714.md](pr-335070-841383d80714.md)

#### Published comment at `src/vs/editor/browser/widget/multiDiffEditor/multiDiffEditorWidget.ts:133`

**[Experimental performance review bot]**

**Severity: medium**

setDiffWordWrap always writes a second fresh \_diffLayoutOptions object. Both SessionsDiffEditorLayoutContribution and SessionChangesEditor call it immediately after setViewMode, so even when neither effective value changed, the same bound entries process another full options update.

With a multi-diff/Changes editor showing many compact entries, switching active editors, changing visible editors, or changing either layout preference synchronously traverses every bound diff twice and updates both inner editors twice, adding avoidable UI-thread work and potential interaction jank.

**Suggested fix:** Expose one setter that applies view mode and word wrap in a single \_diffLayoutOptions.set (and skip the write when the resulting values are unchanged), then use it from both callers; alternatively add equality guards and batch the two fields in one observable transaction.

(Written by Copilot)
