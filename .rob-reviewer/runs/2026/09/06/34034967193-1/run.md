# Reviewer run 34034967193

- Status: **succeeded**
- Started: `2026-09-06T13:04:57Z`
- Completed: `2026-09-06T13:18:23Z`
- Reviewer commit: `563241d87bd04bfd0c3eb92ca9588898ff13175d`
- Bootstrapped: 0
- Reviewed: 5
- Published: 1
- Deferred: 349
- Skipped: 405
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#331691 — Pause automatic inline completions on metered connections](https://github.com/microsoft/vscode/pull/331691)

- Head: `50892dea883d0a5c463219fafe8dca930a9791a7`
- Findings: 0
- Published: false
- Reports: [pr-331691-50892dea883d.json](pr-331691-50892dea883d.json), [pr-331691-50892dea883d.md](pr-331691-50892dea883d.md)

### [#334705 — Make action widget menu items directly clickable by Playwright](https://github.com/microsoft/vscode/pull/334705)

- Head: `2373481a96253a7623f1df8234169b1b000cb224`
- Findings: 1
- Published: true
- Reports: [pr-334705-2373481a9625.json](pr-334705-2373481a9625.json), [pr-334705-2373481a9625.md](pr-334705-2373481a9625.md)

#### Published comment at `src/vs/platform/actionWidget/browser/actionList.ts:961`

**[Experimental performance review bot]**

**Severity: low**

The diff registers List.onMouseMove for the full widget lifetime. Although the callback body becomes a no-op once \_ignoreInitialHover is false, ListView still maps every DOM mousemove through toMouseEvent before invoking it.

After the first movement over an open code-action, dropdown, or model-picker list, all later movements still perform DOM target-to-row resolution and allocate a mapped list event. This scales with mouse polling rate and can add avoidable CPU/GC pressure while the popup is being interacted with.

**Suggested fix:** Use a raw disposable `mousemove` listener on the list element and dispose/clear that listener immediately after the first non-zero movement (while retaining mousedown activation), so row hover continues via the existing `onMouseOver` handler without permanent per-move mapping.

(Written by Copilot)

### [#334756 — eslint: enable no bracket notation rule](https://github.com/microsoft/vscode/pull/334756)

- Head: `0a13539ae890a804f896eabac0a8b4d0739aab6a`
- Findings: 0
- Published: false
- Reports: [pr-334756-0a13539ae890.json](pr-334756-0a13539ae890.json), [pr-334756-0a13539ae890.md](pr-334756-0a13539ae890.md)

### [#334761 — mcp: validate gallery server sources and package types](https://github.com/microsoft/vscode/pull/334761)

- Head: `33e30d7a0266426672781306021b41aa5921d811`
- Findings: 0
- Published: false
- Reports: [pr-334761-33e30d7a0266.json](pr-334761-33e30d7a0266.json), [pr-334761-33e30d7a0266.md](pr-334761-33e30d7a0266.md)

### [#334769 — startup: Cache packaged ESM ASAR resolutions](https://github.com/microsoft/vscode/pull/334769)

- Head: `b4a0bfc933e6c6e7193d4780cb0d0a42b1b6542e`
- Findings: 0
- Published: false
- Reports: [pr-334769-b4a0bfc933e6.json](pr-334769-b4a0bfc933e6.json), [pr-334769-b4a0bfc933e6.md](pr-334769-b4a0bfc933e6.md)
