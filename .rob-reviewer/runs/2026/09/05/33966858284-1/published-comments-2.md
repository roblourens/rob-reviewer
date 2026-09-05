# Published comments: 2

## [#334694 — \[cherry-pick\] agentHost: Preserve workspace transition boundaries](https://github.com/microsoft/vscode/pull/334694) at `src/vs/platform/agentHost/node/chatContributions/sessionWorkspaceConversion/sessionWorkspaceConversionContribution.ts:76`

**[Experimental performance review bot]**

**Severity: low**

Hydrating a converted transcript now performs a fresh storage existence check/database acquisition and a full workspace-transition query after provider history has returned.

Reopening a converted agent session is delayed by an additional filesystem/database round trip after provider history has already completed; sessions with many lazily opened peer or subagent transcripts repeat the delay per transcript.

**Suggested fix:** Read the transition map while the existing restore database reference is open and start that read alongside provider history, then pass the resolved map into hydration. For peer/subagent chats, propagate a storage-specific marker/map so a parent session transition does not trigger empty child-database probes.

(Written by Copilot)

## [#334696 — sessions: measure and unblock V3 onboarding GitHub personalization](https://github.com/microsoft/vscode/pull/334696) at `src/vs/sessions/contrib/chat/browser/newChatInput.ts:685`

**[Experimental performance review bot]**

**Severity: medium**

The PR makes prompt options selectable immediately while repository discovery and GitHub personalization are still in flight, then only suppresses later renders after selection instead of cancelling that work.

A user who immediately chooses one of the newly available standard or partial options can still trigger the rest of a 10-second personalization run in the background. That consumes filesystem and GitHub network capacity after its result can no longer be rendered, potentially contending with session startup and wasting up to eight GraphQL requests per impression.

**Suggested fix:** When option selection begins, cancel the active prompt-options refresh/token in addition to setting the selection guard (without clearing the selected/generated input). Preserve the existing guard for focus-only render suppression, since focus alone should not abort personalization.

(Written by Copilot)

