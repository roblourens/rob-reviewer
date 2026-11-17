# Published comments: 1

## [#348926 — \[Perf\] collections: simplify group accumulation (reviewer test)](https://github.com/microsoft/vscode/pull/348926) at `src/vs/base/common/collections.ts:26`

**[Experimental performance review bot]**

**Severity: medium**

Each insertion allocates a replacement bucket and copies every previously grouped value, changing total grouping work from amortized O(N) to O(N + sum(k\_g^2)) for bucket sizes k\_g.

Searching only changed files in a large change set now performs quadratic CPU work and generates transient arrays before ripgrep starts, delaying the first search result and increasing GC pressure. Warm model/module caches do not avoid this grouping.

**Suggested fix:** Restore one-time bucket initialization followed by in-place push, preserving both group membership and order without repeatedly cloning prefixes.

(Written by Copilot)

