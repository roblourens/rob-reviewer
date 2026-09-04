# Published comments: 1

## [#334375 — Update Component Explorer packages](https://github.com/microsoft/vscode/pull/334375) at `package-lock.json:255`

**[Experimental performance review bot]**

**Severity: medium**

This lockfile refresh deletes every `libc` selector while retaining both GNU and musl packages as optional dependencies with identical Linux OS and CPU constraints, making both variants eligible in the lockfile's install tree.

A Linux x64 cache-miss `npm ci`—including the root install on component-fixtures and CSS-order-scan CI—downloads and unpacks both x64 variants for all three families before any build or fixture can start. Warm node\_modules-cache hits avoid the install, but fresh developer/CI installs pay the extra binary transfer and disk cost.

**Suggested fix:** Restore the deleted `libc` arrays (or regenerate the lockfile with the repository-supported npm version that preserves them) so only the host's GNU or musl artifact is fetched and unpacked.

(Written by Copilot)

