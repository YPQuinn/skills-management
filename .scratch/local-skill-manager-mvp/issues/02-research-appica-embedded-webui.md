# Research Appica's embedded-WebUI constraints

Type: research
Status: resolved
Blocked by:

## Question

What exact Appica project setup, styling and component constraints, build output, asset-base behavior, and client-side routing behavior must Skill Manager account for when embedding the WebUI in a Go executable and serving it through chi?

## Answer

Full brief: [`docs/research/appica-embedded-webui.md`](../../../../docs/research/appica-embedded-webui.md) (researched 2026-08-07 against Appica docs + `@appica/ui-react@1.0.0`).

**Summary:** Appica is a React 19 + Tailwind v4 component package, not an embed framework. Skill Manager needs a client SPA (Vite fits Appica’s docs) that installs `@appica/ui-react`, imports `styles.css`, registers `@source` on the package `dist`, and wraps with `ThemeProvider`. There is no prebuilt component CSS—Tailwind must compile Appica classes at WebUI build time; embed that compiled `dist/`. Routing, asset `base`, SPA fallback, and chi/`embed` wiring are **not** Appica-owned: align Vite `base`, router basename, and chi mount; serve History-API deep links via `index.html` fallback; keep REST routes off that fallback. Compose Appica nav/links onto the chosen router via `render` / variant helpers. Residual gaps: mount path, Browser vs Hash routing, theme policy, CSP vs inline theme script, and `@source` package-name vs relative-path discrepancy across Appica sources.
