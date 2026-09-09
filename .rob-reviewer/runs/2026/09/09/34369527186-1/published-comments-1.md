# Published comments: 1

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410) at `src/vs/platform/agentHost/node/agentService.ts:821`

**[Experimental performance review bot]**

**Severity: medium**

The constructor now schedules reconciliation on a fixed 1-second timer independently of the startup-settled barrier. On a fresh version marker, the pass dirties the whole catalog and reprojects the first 50 sessions.

After upgrading to this schema/version, users with a sizable session history can have up to 50 session databases statted/opened/read and catalog rows verified or rewritten beginning one second after Agent Host construction, while startup or the first session list/load is still in progress. On slower or busy disks this can delay the first useful interaction.

**Suggested fix:** Queue the initial reconciliation through `_runWhenStartupSettled` (or otherwise gate `start`/`schedule` on the same barrier), then begin its periodic schedule after that deferred pass so repair remains automatic without competing with startup.

(Written by Copilot)

