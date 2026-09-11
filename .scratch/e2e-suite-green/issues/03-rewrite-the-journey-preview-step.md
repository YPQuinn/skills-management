# Rewrite the journey spec's desired-set preview step

Type: task
Status: open

## Question

`chromium-journey.spec.ts` clicks `Preview desired Skills` on the Target detail page and then asserts an `All Skills List` dialog. That control no longer exists: `ariaPreviewDesiredSet` is now an unreferenced locale key, left behind when `d8f540f` reworked the Target page into a single Skills list. The click waits out the full 180s test timeout, so the spec fails and everything after it never runs.

## Comments

- Decide first whether the current UI still offers an equivalent preview of a Target's desired Skill set, and through which control. If it does not, the spec should assert whatever now proves the assignment expanded, and the step should be dropped rather than reworded.
- `ariaPreviewDesiredSet` should go from both locale dictionaries once the answer is known; en and zh must stay in lockstep. Check `desiredSetHeading` and its neighbours for the same rot.
- The spec's later steps (Distribute, conflict handling, sync) are believed sound but have not run to completion since at least `aaaabcf`, so expect further drift behind this one.
