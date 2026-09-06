# Published comments: 1

## [#334521 — automations: refactor: make provider session templates canonical](https://github.com/microsoft/vscode/pull/334521) at `src/vs/sessions/contrib/automations/browser/automationDialog.ts:365`

**[Experimental performance review bot]**

**Severity: medium**

Retargeting now synchronously captures the previous provider draft before creating the requested draft; stale retarget iterations still pay that capture and leave the old draft installed, causing the next iteration to capture it again.

When an Agent Host/provider configuration request is slow or disconnected, quickly switching folder/provider/session type leaves the session controls unavailable for roughly two seconds for a common A→B→C correction, and further switches can add another timeout while also spawning additional never-settling capture promises.

**Suggested fix:** Coalesce capture by applied session: start at most one capture attempt for the old draft, share its result/deadline across queued target updates, and after it settles or times out create only the latest requested target. Also cancel or otherwise release the underlying provider capture when abandoning the draft.

(Written by Copilot)

