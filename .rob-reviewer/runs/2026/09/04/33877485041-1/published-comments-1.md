# Published comments: 1

## [#334022 — Center full-width characters in two monospace cells](https://github.com/microsoft/vscode/pull/334022) at `src/vs/editor/common/viewModel/monospaceLineBreaksComputer.ts:46`

**[Experimental performance review bot]**

**Severity: medium**

With forceFullwidthCharacterWidth enabled, the added condition rejects prior projection data unconditionally and sends every wrapped line through createLineBreaks, which scans the complete line and allocates fresh break-offset arrays and a new ModelLineProjectionData.

In a large CJK/Markdown document with viewport word wrap and this new option enabled, dragging a panel or resizing the editor synchronously reconstructs all model lines on every wrapping-column update, causing avoidable UI stalls and GC pressure.

**Suggested fix:** Retain the previous-data branch and call createLineBreaksFromPreviousLineBreaks with `forceFullwidthCharacterWidth ? 2 : columnsForFullWidthChar`; add a resize/previousLineBreakData test with the option enabled to preserve the exact two-column semantics.

(Written by Copilot)

