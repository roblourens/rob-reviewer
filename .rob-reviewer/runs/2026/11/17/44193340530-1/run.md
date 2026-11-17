# Reviewer run 44193340530

- Status: **succeeded**
- Started: `2026-11-17T22:47:31Z`
- Completed: `2026-11-17T22:50:37Z`
- Reviewer commit: `7352677cf6c3512a27edc6b7a0e826158b363179`
- Bootstrapped: 0
- Reviewed: 1
- Published: 1
- Deferred: 0
- Skipped: 0
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#348926 — \[Perf\] collections: simplify group accumulation (reviewer test)](https://github.com/microsoft/vscode/pull/348926)

- Head: `867800d27f1b734c551ab5e88a4e622d23ed5692`
- Findings: 1
- Published: true
- Reports: [pr-348926-867800d27f1b.json](pr-348926-867800d27f1b.json), [pr-348926-867800d27f1b.md](pr-348926-867800d27f1b.md)

#### Published comment at `src/vs/base/common/collections.ts:26`

**[Experimental performance review bot]**

**Severity: medium**

Each insertion allocates a replacement bucket and copies every previously grouped value, changing total grouping work from amortized O(N) to O(N + sum(k\_g^2)) for bucket sizes k\_g.

Searching only changed files in a large change set now performs quadratic CPU work and generates transient arrays before ripgrep starts, delaying the first search result and increasing GC pressure. Warm model/module caches do not avoid this grouping.

**Suggested fix:** Restore one-time bucket initialization followed by in-place push, preserving both group membership and order without repeatedly cloning prefixes.

(Written by Copilot)
