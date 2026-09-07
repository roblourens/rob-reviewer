# Published comments: 1

## [#334889 — Sort chat pill dropdown entries newest first](https://github.com/microsoft/vscode/pull/334889) at `src/vs/workbench/contrib/browserView/common/browserView.ts:113`

**[Experimental performance review bot]**

**Severity: medium**

The shared newest-first helper inserts every match at index zero, shifting the accumulated array for each matching browser on every recomputation.

When an agent session accumulates many browser pages, each page load/navigation emits several label events; both pill surfaces can repeatedly perform quadratic array copying before updating the UI, delaying rendering and increasing GC pressure.

**Suggested fix:** Append matching inputs with `push` during the map walk and call `views.reverse()` once afterward; this preserves newest-first ordering and restores O(n) construction.

(Written by Copilot)

