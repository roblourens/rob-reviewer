# Reviewer run 33921409256

- Status: **succeeded**
- Started: `2026-09-04T21:31:23Z`
- Completed: `2026-09-04T21:42:03Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 3
- Published: 2
- Deferred: 349
- Skipped: 391
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `c607b06d6dbec2888b4ff59bd366acfd883919e3`
- Findings: 1
- Published: true
- Reports: [pr-332410-c607b06d6dbe.json](pr-332410-c607b06d6dbe.json), [pr-332410-c607b06d6dbe.md](pr-332410-c607b06d6dbe.md)

#### Published comment at `src/vs/platform/agentHost/node/agentService.ts:749`

**[Experimental performance review bot]**

**Severity: high**

The new unconditional callback sends every `onDidChangeSessionSummary` event to `_queueCatalogSync`. That method immediately creates and retains a distinct `_persistListVisibleSessionStateNow` promise; it does not merge an update into an existing pending write.

A tool-rich agent turn produces repeated summary status/activity/timestamp changes (the notifier can flush every 100 ms), and concurrent active sessions multiply them. Every change opens `session.db`, reads all catalog metadata, canonicalizes/hashes the full payload, writes the local pending snapshot and compatibility metadata, upserts `agent-host.db`, acknowledges the local snapshot, and marks the central row dirty. These writes are serialized per session and host-wide, so bursts can build a disk-I/O backlog and delay list/registry work or deletion, which explicitly waits for all background catalog writes.

**Suggested fix:** Keep at most one in-flight catalog write and one merged trailing update per session. Merge metadata overrides and rebuild the trailing projection from the latest state when the in-flight write settles; preserve the existing ordered/teardown flush paths by awaiting that coalesced drain.

(Written by Copilot)

### [#334022 — Center full-width characters in two monospace cells](https://github.com/microsoft/vscode/pull/334022)

- Head: `b9b8ede3afdb676101dccab70bb672660762b57f`
- Findings: 1
- Published: true
- Reports: [pr-334022-b9b8ede3afdb.json](pr-334022-b9b8ede3afdb.json), [pr-334022-b9b8ede3afdb.md](pr-334022-b9b8ede3afdb.md)

#### Published comment at `src/vs/editor/browser/view/domLineBreaksComputer.ts:280`

**[Experimental performance review bot]**

**Severity: high**

Advanced wrapping now materializes each full-width character as its own styled DOM element whenever the new two-cell option is active, instead of keeping ordinary text in large shared spans.

Opening, reconfiguring, resizing/re-wrapping, or flushing a large CJK-heavy editor with `fullwidthCharacterWidth: 'twoCells'` and advanced wrapping can block the UI while the browser parses, styles, lays out, and measures hundreds of thousands of temporary elements; peak DOM memory and GC pressure grow at the same time.

**Suggested fix:** Keep measurement semantics but bound the fan-out: process queued lines/characters in capped batches so only a limited temporary subtree is live at once, and where possible compute fixed two-cell runs outside the browser instead of emitting one measurement element per character.

(Written by Copilot)

### [#334382 — Fix permission bits in mock-policy-server file commands](https://github.com/microsoft/vscode/pull/334382)

- Head: `6eed6b35cdadbddb3602aba79f71c0d7ebfb293c`
- Findings: 0
- Published: false
- Reports: [pr-334382-6eed6b35cdad.json](pr-334382-6eed6b35cdad.json), [pr-334382-6eed6b35cdad.md](pr-334382-6eed6b35cdad.md)
