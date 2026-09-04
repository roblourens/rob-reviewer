# Published comments: 1

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410) at `src/vs/platform/agentHost/node/agentHostPeerChatStore.ts:339`

**[Experimental performance review bot]**

**Severity: medium**

\_publishCompatibilityState now maps the complete current entry list to \_writeChatMetadata before writing the parent legacy mirror, and createChat/disposeChat synchronously await peerChatStore.upsert/remove.

In a long-lived session with many peer chats, adding or deleting one chat blocks on potentially hundreds of unrelated SQLite opens/writes (250 four-wide waves at the 1,000-chat limit). Repeated additions accumulate roughly N(N+1)/2 writes, causing increasingly slow chat operations and disk churn.

**Suggested fix:** Diff the previous and updated central catalogs and write chat-local metadata only for added or actually changed entries, then write the parent peerChats mirror once. Reserve full per-chat repair sweeps for migration/background reconciliation rather than the interactive mutation path.

(Written by Copilot)

