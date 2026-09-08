# Published comments: 3

## [#334513 — feat: implementing word wrap indicators](https://github.com/microsoft/vscode/pull/334513) at `src/vs/editor/browser/viewParts/wordWrapIndicator/wordWrapIndicator.ts:108`

**[Experimental performance review bot]**

**Severity: medium**

The new overlay calls ctx.viewportData.getViewLineRenderingData for every visible line on each invalidation even though it reads only continuesWithWrappedLine.

With wordWrapIndicator enabled, a tall wrapped editor containing many visible decorations (for example search matches, diagnostics, and inline decorations) repeats the viewport-decoration scan once per visible line whenever scrolling or decoration changes trigger rendering; hundreds of visible lines and decorations can therefore produce tens of thousands of checks on a frame and cause dropped frames.

**Suggested fix:** Read continuesWithWrappedLine from this.\_context.viewModel.getViewLineData(lineNumber) (or add an equally narrow accessor) so the pass remains O(V) and avoids allocating full rendering data.

(Written by Copilot)

## [#335053 — agentHost: surface Codex skill validation errors](https://github.com/microsoft/vscode/pull/335053) at `src/vs/platform/agentHost/node/codex/codexCustomizations.ts:362`

**[Experimental performance review bot]**

**Severity: medium**

Invalid skill paths are now projected as E separate top-level containers, which the existing refresh publisher emits as E independent state updates.

When a workspace contains many invalid or legacy skills (for example, a collection whose SKILL.md files all omit the newly required description), every coalesced `skills/changed` refresh now publishes one `SessionCustomizationUpdated` action per invalid file. Those actions are processed synchronously and individually, so repairing or editing skills can cause noticeable Customizations UI churn and agent-host IPC/CPU spikes.

**Suggested fix:** Preserve the visible per-skill diagnostics but batch their state publication—for example, add a directory-customization batch replacement action—or group children that share the same diagnostic into one error container so a common schema failure does not create one top-level update per file.

(Written by Copilot)

## [#335070 — Add word wrap control to Agents editors](https://github.com/microsoft/vscode/pull/335070) at `src/vs/editor/browser/widget/multiDiffEditor/multiDiffEditorWidget.ts:133`

**[Experimental performance review bot]**

**Severity: medium**

setDiffWordWrap always writes a second fresh \_diffLayoutOptions object. Both SessionsDiffEditorLayoutContribution and SessionChangesEditor call it immediately after setViewMode, so even when neither effective value changed, the same bound entries process another full options update.

With a multi-diff/Changes editor showing many compact entries, switching active editors, changing visible editors, or changing either layout preference synchronously traverses every bound diff twice and updates both inner editors twice, adding avoidable UI-thread work and potential interaction jank.

**Suggested fix:** Expose one setter that applies view mode and word wrap in a single \_diffLayoutOptions.set (and skip the write when the resulting values are unchanged), then use it from both callers; alternatively add equality guards and batch the two fields in one observable transaction.

(Written by Copilot)

