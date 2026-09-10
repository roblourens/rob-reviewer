# Published comments: 1

## [#333185 — editor: Balance auto-closing brackets](https://github.com/microsoft/vscode/pull/333185) at `src/vs/editor/common/cursor/cursorTypeEditOperations.ts:253`

**[Experimental performance review bot]**

**Severity: medium**

An opening-bracket keystroke now requests the bracket-pair tree. On a cold model this synchronously tokenizes/parses the entire document into an AST and retains it, before the query can decide whether token-aware data is usable.

With bracket-pair colorization/guides disabled (or another editor/model where no bracket consumer has run), the first `{`, `[`, or `(` in a multi-megabyte file can pause typing while up to 5 MB is parsed and a proportional AST is allocated and retained. This defeats the responsiveness benefit of disabling bracket visualization; the waste is especially clear while background tokenization is incomplete because the result is not used.

**Suggested fix:** Keep whole-document tree creation off the keystroke path: query only an already-available token-aware tree and schedule/build it outside input handling, or at least check full-model token accuracy and tree availability before setting bracketsRequested/updateBracketPairsTree so an unusable AST is not constructed synchronously.

(Written by Copilot)

