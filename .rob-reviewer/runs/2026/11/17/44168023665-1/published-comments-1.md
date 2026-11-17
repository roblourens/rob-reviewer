# Published comments: 1

## [#348909 — \[Perf\] arrays: adjust duplicate tracking (reviewer test)](https://github.com/microsoft/vscode/pull/348909) at `src/vs/base/common/arrays.ts:406`

**[Experimental performance review bot]**

**Severity: medium**

distinct now linearly scans accumulated unique keys for every input element, turning expected O(n) duplicate tracking into O(n\*u), quadratic when most keys are unique.

Filtering Settings with &#64;modified or a broad search, and opening/rebuilding Keyboard Shortcuts, now perform quadratic duplicate checks on potentially thousands of mostly unique setting or command keys on the renderer thread, delaying results and blocking UI responsiveness even when the visible filtered result is small.

**Suggested fix:** Keep Set-based membership tracking (a Set&lt;unknown&gt; preserves the intended typing); retain the existing filter order and keyFn invocation semantics.

(Written by Copilot)

