# Realign the a11y spec's post-registration assertion with the Source page

Type: task
Status: resolved

## Question

`e2e/webui/a11y.spec.ts` fails on `main`, before and independently of any Overview tab work. After clicking `Register and scan` it waits for a heading matching `/a11y|skills/i`, but the Source detail page it lands on renders `upstream` as its `h1` and `Inventory (1)` as its `h2`. Neither matches, so the expectation times out and the axe sweep never runs on the Source, Skills, Groups, and Targets pages.

The assertion went stale when the Source page was reworked: the spec was last touched in `a8f30b6` (2026-09-09) and `internal/webui/src/source-detail.tsx` in `d8f540f` (2026-09-10).

## Comments

- Every sibling spec asserts the registered Source's own heading: `smoke.spec.ts` waits for `smoke-src`, `chromium-journey.spec.ts` for `e2e-local`, `keyboard.spec.ts` for `key-src`. The a11y spec's Source root is `<home>/upstream`, so its heading is `upstream`.
- `smoke.spec.ts` also proves the Inventory arrived by asserting the entry's relative directory text (`skills/smoke`). The same evidence works here (`skills/a11y`) and is stronger than a name-or-anything regex.
- The fix must keep the axe sweeps that follow; the point of the wait is only to reach a settled page before analyzing it.

## Answer

Replaced the stale regex wait with the two facts the page actually shows after registration: the Source heading `upstream` and the Inventory entry `skills/a11y`. This matches the sibling specs and lets the axe sweeps that follow run again.

Unblocking the wait uncovered a pre-existing defect the timeout had been hiding: the axe sweep on the Skill detail page reports one serious `color-contrast` violation on the inactive Tabs trigger, `4.47` where AA needs `4.5` (foreground `#364153` over a blended `#a6abb5`, 14px normal). It reproduces identically on a worktree built from `aaaabcf` with only this assertion patched, so it is not caused by any Overview tab work. The spec therefore still fails; fixing the token or the decorative layer beneath the tab bar is a separate decision and is not done here.

Two more specs are red on `aaaabcf` for one unrelated reason: `chromium-journey.spec.ts` and `keyboard.spec.ts` both wait on a bare `getByRole('dialog')` while an `Import finished` toast is still open, and Appica's toast also carries `role="dialog"`, so the locator resolves to two elements. Untouched here.
