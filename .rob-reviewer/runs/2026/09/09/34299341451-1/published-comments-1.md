# Published comments: 1

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410) at `src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:5342`

**[Experimental performance review bot]**

**Severity: medium**

Tool completion callbacks are now tracked in one set for the lifetime of the session wrapper, and every normal idle waits for all entries in that set, regardless of which turn created them.

After aborting an edit and immediately sending a replacement prompt, the replacement turn can appear stuck at completion until unrelated cleanup from the cancelled turn finishes, increasing end-of-turn latency in proportion to the slowest outstanding prior edit.

**Suggested fix:** Track completion promises by their captured turn and await only the current turn's set. On abort, detach that turn's entries from the idle barrier while still allowing their cleanup/persistence promises to settle.

(Written by Copilot)

