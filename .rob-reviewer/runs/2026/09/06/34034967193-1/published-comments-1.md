# Published comments: 1

## [#334705 — Make action widget menu items directly clickable by Playwright](https://github.com/microsoft/vscode/pull/334705) at `src/vs/platform/actionWidget/browser/actionList.ts:961`

**[Experimental performance review bot]**

**Severity: low**

The diff registers List.onMouseMove for the full widget lifetime. Although the callback body becomes a no-op once \_ignoreInitialHover is false, ListView still maps every DOM mousemove through toMouseEvent before invoking it.

After the first movement over an open code-action, dropdown, or model-picker list, all later movements still perform DOM target-to-row resolution and allocate a mapped list event. This scales with mouse polling rate and can add avoidable CPU/GC pressure while the popup is being interacted with.

**Suggested fix:** Use a raw disposable `mousemove` listener on the list element and dispose/clear that listener immediately after the first non-zero movement (while retaining mousedown activation), so row hover continues via the existing `onMouseOver` handler without permanent per-move mapping.

(Written by Copilot)

