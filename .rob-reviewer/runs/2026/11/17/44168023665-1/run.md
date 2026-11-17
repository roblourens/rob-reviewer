# Reviewer run 44168023665

- Status: **succeeded**
- Started: `2026-11-17T20:19:57Z`
- Completed: `2026-11-17T20:22:42Z`
- Reviewer commit: `7352677cf6c3512a27edc6b7a0e826158b363179`
- Bootstrapped: 0
- Reviewed: 1
- Published: 1
- Deferred: 0
- Skipped: 0
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#348909 — \[Perf\] arrays: adjust duplicate tracking (reviewer test)](https://github.com/microsoft/vscode/pull/348909)

- Head: `b756a399a0c46b056202f37a77e60d185f6db3fb`
- Findings: 1
- Published: true
- Reports: [pr-348909-b756a399a0c4.json](pr-348909-b756a399a0c4.json), [pr-348909-b756a399a0c4.md](pr-348909-b756a399a0c4.md)

#### Published comment at `src/vs/base/common/arrays.ts:406`

**[Experimental performance review bot]**

**Severity: medium**

distinct now linearly scans accumulated unique keys for every input element, turning expected O(n) duplicate tracking into O(n\*u), quadratic when most keys are unique.

Filtering Settings with &#64;modified or a broad search, and opening/rebuilding Keyboard Shortcuts, now perform quadratic duplicate checks on potentially thousands of mostly unique setting or command keys on the renderer thread, delaying results and blocking UI responsiveness even when the visible filtered result is small.

**Suggested fix:** Keep Set-based membership tracking (a Set&lt;unknown&gt; preserves the intended typing); retain the existing filter order and keyFn invocation semantics.

(Written by Copilot)
