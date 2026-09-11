# Keep the keyboard spec inside its time budget

Type: task
Status: open

## Question

`keyboard.spec.ts` exhausts the 180s test timeout. It reaches its assignment step, so nothing is logically broken; the budget simply goes into `tabTo`, which presses Tab up to 80 times and pays a browser round trip per press to ask whether the target is focused. The spec calls it for nearly every interaction, and each added focusable element in the header and navigation makes every call longer.

## Comments

- The helper's contract is worth keeping: a keyboard-only spec should reach controls the way a keyboard user does, so replacing Tab with `focus()` would gut the test.
- Cheaper options: read `document.activeElement` once per press inside a single evaluate that also counts presses, start the walk from a known anchor instead of wherever focus happens to be, or give this one spec a longer timeout because it is inherently serial.
- Measure before choosing. The failure appeared only after the dialog and Escape fixes let the spec run to its later steps, so the current cost per `tabTo` call is unknown.
