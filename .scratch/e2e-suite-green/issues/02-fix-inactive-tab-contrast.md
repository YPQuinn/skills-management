# Fix the inactive Tabs trigger contrast on the Skill detail page

Type: task
Status: resolved

## Question

The axe sweep on the Skill detail page reports one serious `color-contrast` violation: the inactive Tabs trigger renders `#364153` on a blended `#a6abb5` for a ratio of `4.47`, where WCAG AA needs `4.5` for 14px normal text. It reproduces on a worktree built from `aaaabcf`, so it predates the Overview tab work; it only became visible once `a11y.spec.ts` stopped timing out earlier in the flow.

## Comments

- The blended background comes from the stack under the tab bar, not from a solid token, so the first step is to identify in the browser which layer contributes `#a6abb5`.
- Appica owns the Tabs styling. The `01-fix-dark-mode-tab-contrast` decision already removed a `!text-black` override, so the fix must not reintroduce a hand-picked colour: prefer an Appica-sanctioned prop or an opaque surface under the tab bar.
- Verification is the real axe run (`e2e/webui/a11y.spec.ts`), not a hand-rolled contrast script; the ratio is 0.03 short, so eyeballing cannot confirm it.
- Both light and dark themes must stay above threshold.
- Measured in the browser: the surface actually behind the tab label is `oklch(1 0 0)`, plain white, and the label is `#364153`, so the real ratio is about 10:1. The `#a6abb5` axe reports is the blend of white with `background-pattern-layer` at `0.58` alpha of `--pattern-color` (`--color-border-intense`, `rgb(106 114 130)`).
- That 0.58 layer is a child of `background-pattern-highlight`, whose computed opacity is `0` until the pointer arrives; dispatching a pointer event over the tab bar fades it toward `0.286`. axe ignores the ancestor opacity and treats the dot layer as a solid fill, so the report is pessimistic — but near the cursor the dots do sit behind the label, so the concern is not purely theoretical.
- Appica exposes no strength knob for the spotlight: `spotlight` takes only `true`, a size, or `{ size, persistent }`. The documented lever is the `--pattern-color` variable, and the docs put the burden on the caller: "The pattern is intentionally low-emphasis; ensure your foreground content keeps sufficient contrast."
- So the options are a lighter `--pattern-color` on the `BackgroundPattern` in `layout.tsx`, dropping `spotlight` altogether, or accepting the finding as an axe limitation. Each changes how the app looks or what the suite enforces, so it needs the operator's call.

## Answer

Dropped `spotlight` from the `BackgroundPattern` in `layout.tsx`. The base dot layer stays, so the page keeps its texture; what goes away is the cursor-following highlight whose brightened dots sat behind page text. `a11y.spec.ts` now passes end to end, with axe clean on the setup, Skills, Source, Skill detail, Groups, and Targets pages.

Two things followed from the removal:

- `layout.test.tsx` pinned `spotlight` to `true`, so it now pins the opposite and records why.
- `index.css` clipped both decorative layers to the panel's rounded corners; the `background-pattern-highlight` selector no longer matches anything and was dropped.

`responsive-theme.spec.ts` still guards the corners against leaking dots and still passes, though its pointer movement no longer exercises a spotlight — it now covers only the base layer.
