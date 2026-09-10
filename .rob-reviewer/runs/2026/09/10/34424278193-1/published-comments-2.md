# Published comments: 2

## [#335366 — chat: Show MCP server error details in customizations](https://github.com/microsoft/vscode/pull/335366) at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:383`

**[Experimental performance review bot]**

**Severity: medium**

The renderer now assigns the complete raw error message to textContent and the accompanying CSS uses pre-wrap, overflow-wrap:anywhere, and visible overflow, so no display or measurement bound remains.

Opening Customizations after an HTTP MCP endpoint returns a large error body, or after an SDK reports verbose startup diagnostics, can freeze or severely jank the editor while that full payload is wrapped and measured.

**Suggested fix:** Cap the inline/ARIA preview by bytes or lines before DOM assignment and constrain row height, while preserving access to the complete diagnostic through Show Output or an explicit on-demand expansion that performs the large layout only when requested.

(Written by Copilot)

## [#335366 — chat: Show MCP server error details in customizations](https://github.com/microsoft/vscode/pull/335366) at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:1747`

**[Experimental performance review bot]**

**Severity: medium**

A new section autorun reads every cached row label on any shared customization/session change, marks the whole list for remeasurement, and schedules rerender.

With many installed MCP servers, startup, failure, recovery, or enablement updates can repeatedly delay interaction and rendering on the Customizations page.

**Suggested fix:** Leave label subscriptions with rendered accessibility rows. Use one shared status/session subscription to request layout, and invalidate only rows whose error text or visibility changed rather than eagerly evaluating all labels and rerendering the whole list.

(Written by Copilot)

