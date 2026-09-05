# Reviewer run 33966858284

- Status: **succeeded**
- Started: `2026-09-05T12:44:24Z`
- Completed: `2026-09-05T12:58:15Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 3
- Published: 2
- Deferred: 347
- Skipped: 391
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334694 — \[cherry-pick\] agentHost: Preserve workspace transition boundaries](https://github.com/microsoft/vscode/pull/334694)

- Head: `4b4be225afb770efd6718857998959ecc9d1f6ce`
- Findings: 1
- Published: true
- Reports: [pr-334694-4b4be225afb7.json](pr-334694-4b4be225afb7.json), [pr-334694-4b4be225afb7.md](pr-334694-4b4be225afb7.md)

#### Published comment at `src/vs/platform/agentHost/node/chatContributions/sessionWorkspaceConversion/sessionWorkspaceConversionContribution.ts:76`

**[Experimental performance review bot]**

**Severity: low**

Hydrating a converted transcript now performs a fresh storage existence check/database acquisition and a full workspace-transition query after provider history has returned.

Reopening a converted agent session is delayed by an additional filesystem/database round trip after provider history has already completed; sessions with many lazily opened peer or subagent transcripts repeat the delay per transcript.

**Suggested fix:** Read the transition map while the existing restore database reference is open and start that read alongside provider history, then pass the resolved map into hydration. For peer/subagent chats, propagate a storage-specific marker/map so a parent session transition does not trigger empty child-database probes.

(Written by Copilot)

### [#334696 — sessions: measure and unblock V3 onboarding GitHub personalization](https://github.com/microsoft/vscode/pull/334696)

- Head: `fda8f5b56fbe33ab077346719a380e643ffe3b49`
- Findings: 1
- Published: true
- Reports: [pr-334696-fda8f5b56fbe.json](pr-334696-fda8f5b56fbe.json), [pr-334696-fda8f5b56fbe.md](pr-334696-fda8f5b56fbe.md)

#### Published comment at `src/vs/sessions/contrib/chat/browser/newChatInput.ts:685`

**[Experimental performance review bot]**

**Severity: medium**

The PR makes prompt options selectable immediately while repository discovery and GitHub personalization are still in flight, then only suppresses later renders after selection instead of cancelling that work.

A user who immediately chooses one of the newly available standard or partial options can still trigger the rest of a 10-second personalization run in the background. That consumes filesystem and GitHub network capacity after its result can no longer be rendered, potentially contending with session startup and wasting up to eight GraphQL requests per impression.

**Suggested fix:** When option selection begins, cancel the active prompt-options refresh/token in addition to setting the selection guard (without clearing the selected/generated input). Preserve the existing guard for focus-only render suppression, since focus alone should not abort personalization.

(Written by Copilot)

### [#334702 — chat: always enable turn status pills](https://github.com/microsoft/vscode/pull/334702)

- Head: `f416c523c7a7c38e1db27282a700ebb96d92232a`
- Findings: 0
- Published: false
- Reports: [pr-334702-f416c523c7a7.json](pr-334702-f416c523c7a7.json), [pr-334702-f416c523c7a7.md](pr-334702-f416c523c7a7.md)
