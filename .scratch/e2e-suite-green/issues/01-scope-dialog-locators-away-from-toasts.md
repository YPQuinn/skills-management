# Stop dialog waits from resolving to open toasts

Type: task
Status: claimed

## Question

`chromium-journey.spec.ts` and `keyboard.spec.ts` both fail on `main` with the same strict-mode violation: `getByRole('dialog')` resolves to two elements, because Appica's toast carries `role="dialog"` (with `aria-modal="false"`) and an `Import finished` toast is still open when the next dialog is opened. Every bare `getByRole('dialog')` in the suite is ambiguous once a toast is on screen.

## Comments

- The toast is deliberately sticky: the `14-run-outcomes-belong-in-the-toast` decision made run outcomes wait to be dismissed. So the toast is not the bug; the locator is.
- The trigger name and the dialog's accessible name differ (trigger `Create Group`, dialog `Create a Group`), so the fix cannot derive the dialog name from the trigger.
- These specs already locate Appica internals by slot (`[data-slot="background-pattern"]`, `[data-slot="navigation-list"]`, `[data-slot="toast"]`), so scoping to `[data-slot="dialog-popup"]` follows the existing convention.
- The shared `openCreateDialog` helper carries the same bare locator and must be fixed with the call sites.

## Answer

`helpers.ts` gained `dialogPopup(page)`, scoped to `[data-slot="dialog-popup"]`, and every bare `getByRole('dialog')` in `chromium-journey.spec.ts`, `keyboard.spec.ts`, and `openCreateDialog` now goes through it. Locators that already name their dialog, and the `alertdialog` ones, were left alone. The helper is named `dialogPopup` rather than `dialog` because `keyboard.spec.ts` declares a local `const dialog` for its confirm dialog, which would shadow the import for the whole test body.

Clearing this unmasked two further breakages, both fixed here because the spec cannot reach its later steps otherwise:

- `dismissOpenListbox` raced the Select popup's closing animation. The Playwright trace shows `isVisible` on the listbox returning true, then `Escape` landing on the dialog behind the just-closed popup and dismissing it, which left the keyboard spec tabbing around the bare Targets page. It now waits the popup out for up to a second and only presses `Escape` if the listbox truly stays open.
- The Group detail page renders its member Skills as a card grid, not a table, so `getByRole('table').getByRole('link', ...)` could never match. It now asserts the member link directly.

Both specs still exhaust the 180s test timeout, for reasons that are out of this ticket's scope: the journey spec waits three minutes on `Preview desired Skills`, a control the Target page no longer has (`ariaPreviewDesiredSet` is now an unreferenced locale key), and the keyboard spec spends its budget in `tabTo`, which presses Tab up to 80 times with a round trip per press.
