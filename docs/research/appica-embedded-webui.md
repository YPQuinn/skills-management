# Appica embedded-WebUI constraints

**Research date:** 2026-08-07  
**Ticket:** `.scratch/local-skill-manager-mvp/issues/02-research-appica-embedded-webui.md`  
**Question:** What exact Appica project setup, styling and component constraints, build output, asset-base behavior, and client-side routing behavior must Skill Manager account for when embedding the WebUI in a Go executable and serving it through chi?

## How to read this brief

Claims are split into two layers:

1. **Appica-guaranteed** — stated by Appica docs, the published package, or Appica’s live site.
2. **Integration implication** — not owned by Appica; derived from the build tool (Vite), router, Go `embed`, or chi when embedding a static SPA. These are recommendations for Skill Manager’s intended architecture (local chi service + one executable), not Appica APIs.

Do **not** treat integration implications as Appica requirements. Product choices (mount path, router style, theme defaults, API prefix) remain open unless marked decided elsewhere.

## Sources

### Primary (Appica)

| Source | Role |
| --- | --- |
| Local `docs/design/llms.md` | Appica topic index (project copy) |
| Local `docs/design/llms-full.md` | Full Appica docs dump used for exact API/text |
| https://appica.dev/ui/docs | Live Appica UI docs |
| https://appica.dev/ui/docs/react/installation | Installation |
| https://appica.dev/ui/docs/react/usage | Imports, styling, composition, client stacks |
| https://appica.dev/ui/docs/react/composition | `render` prop / router composition |
| https://appica.dev/ui/docs/react/theming | Design tokens |
| https://appica.dev/ui/docs/react/dark-mode | Class-based dark mode |
| https://appica.dev/ui/docs/react/theme-provider | `ThemeProvider` API |
| https://appica.dev/ui/docs/react/accessibility | A11y contract |
| https://appica.dev/ui/docs/react/fonts | Font tokens / loading |
| https://appica.dev/ui/components/react/navigation | Navigation + router projection |
| https://appica.dev/ui/icons | Icons package |
| npm `@appica/ui-react@1.0.0` (`package.json`, `README.md`, `styles.css`, dist) | Exact package surface |
| npm `@appica/icons-react@1.0.0` (`README.md`, peer deps) | Icons package surface |

### Secondary (build / serve only where Appica does not own the behavior)

| Source | Role |
| --- | --- |
| https://vite.dev/guide/build.html | Production build, public base path |
| https://vite.dev/config/shared-options.html | `base`, `publicDir` |
| https://vite.dev/config/build-options.html | `outDir`, `assetsDir` |
| https://vite.dev/guide/static-deploy.html | Static hosting / SPA rewrite examples |
| https://tailwindcss.com/docs/functions-and-directives | `@source` directive |
| https://tailwindcss.com/docs/detecting-classes-in-source-files | Why `node_modules` must be registered |
| https://pkg.go.dev/embed | Go embed directives |
| https://pkg.go.dev/net/http#FileServer | Static file serving contract |

## Version snapshot (as of research date)

| Package | Version observed | Notes |
| --- | --- | --- |
| `@appica/ui-react` | **1.0.0** (only published version found; npm time `2026-07-09`) | `"type": "module"`; `engines.node: ">=20"`; `sideEffects: ["./styles.css"]`; ships `dist/`, `src/`, `styles.css` |
| `@appica/icons-react` | **1.0.0** | Separate package; React 19 peer; ESM only |
| Peer deps (`@appica/ui-react`) | `react ^19`, `react-dom ^19`, `tailwindcss ^4.0`, `@types/react ^19` | Matches docs: React **≥ 19**, Tailwind **≥ 4.0** |
| Runtime deps of note | `@base-ui/react ^1.6.0`, `motion`, `tailwind-merge`, `class-variance-authority`, date/carousel helpers | Bundled with the library; not separate install steps in Appica docs |

**Uncertainty:** Only `1.0.0` exists on npm today. Future minors may change `@source` guidance (see discrepancy below). Pin and re-check package README on upgrade.

---

## Executive answer

Appica is a **plain React + Tailwind v4 component package**, not an application framework and not a static-site or embed toolkit. Skill Manager must:

1. Build a **React 19 client app** whose Tailwind pipeline compiles Appica’s class names (there is **no** drop-in prebuilt component CSS).
2. Import `@appica/ui-react/styles.css`, register the package with Tailwind `@source`, and wrap the tree in `ThemeProvider`.
3. Prefer **subpath imports** and compose navigation with the **`render` prop** (or variant helpers) onto whatever router Skill Manager chooses.
4. Treat **build output, asset base path, SPA history fallback, and chi/`embed` wiring as non-Appica concerns** owned by the chosen bundler (Vite is the Appica-documented client SPA path) and the Go server.

Appica does **not** prescribe Go, chi, `embed`, mount paths, BrowserRouter vs HashRouter, or REST layout.

---

## 1. Project setup and package requirements

### 1.1 What Appica is (guaranteed)

From `docs/design/llms.md` and Installation:

- Open-source **accessible React component library** built on **Base UI** and **Tailwind CSS v4**, plus optional **Appica Icons** and Country Flags packages.
- Ships as a **single tree-shakeable package** (`@appica/ui-react`).
- Installation is two steps: **add the package**, then **point Tailwind at it**.

Live: https://appica.dev/ui/docs/react/installation  
Local: `docs/design/llms-full.md` → `# Installation (/ui/docs/react/installation)`

### 1.2 Hard prerequisites (guaranteed)

| Dependency | Requirement | Why it matters |
| --- | --- | --- |
| React | `>= 19` | “Hard requirement”: modern ref-as-prop; no `forwardRef` shims |
| React DOM | `>= 19` | Peer of the package |
| Tailwind CSS | `>= 4.0` | Appica does **not** bundle Tailwind; **your** Tailwind compiles component utilities |
| Node (package engines) | `>= 20` | From `@appica/ui-react@1.0.0` `engines` |
| Module format | ESM (`"type": "module"`) | Package exports are `import` conditional exports |

Install:

```bash
npm install @appica/ui-react
# optional icons used throughout Appica examples:
npm install @appica/icons-react
```

Icons: https://appica.dev/ui/icons — separate package, React 19+, named ESM exports, tree-shakeable.

### 1.3 Tailwind must work before Appica (guaranteed)

Installation warning: set up Tailwind v4 **first** (`tailwindcss` **and** the build plugin: `@tailwindcss/vite` for Vite, `@tailwindcss/postcss` for PostCSS/Next). Confirm `@import 'tailwindcss';` generates utilities. If Tailwind is not compiling, Appica components render **unstyled**.

### 1.4 Global stylesheet contract (guaranteed)

Documented consumer CSS:

```css
@import 'tailwindcss';
@import '@appica/ui-react/styles.css';

@source '../node_modules/@appica/ui-react/dist';
```

Facts:

- `@appica/ui-react/styles.css` is a real package export (`exports["./styles.css"]`).
- It brings the **token system** (colors, radii, shadows, typography, light/dark themes) — not a prebuilt catalog of every component’s final CSS.
- There is **no** prebuilt component stylesheet to import. Component class names ship as source for **your** Tailwind to generate. Benefits Appica cites: one deduplicated build, tree-shaken utilities, token overrides flow into components, no version-locked frozen CSS.
- `@source` is **mandatory**. Tailwind ignores `node_modules` by default; without `@source`, components render unstyled.
- `@source` path is **relative to the stylesheet that contains it**, not a bare package name, according to the local Installation dump and Tailwind’s own docs example (`@source "../node_modules/@my-company/ui-lib"`).

Package contents relevant to scanning:

- Published files include `dist/`, `src/`, and `styles.css`.
- Installation’s canonical scan target is **`.../@appica/ui-react/dist`**.

#### `@source` syntax discrepancy (record explicitly)

| Source | Guidance |
| --- | --- |
| Local `llms-full.md` Installation | Use a **real relative path** to `dist`. Warns: `@source '@appica/ui-react'` “does not resolve and silently scans nothing.” |
| `@appica/ui-react@1.0.0` README | Shows `@source '@appica/ui-react';` and says: on **Tailwind &lt; 4.2**, use relative `@source '../node_modules/@appica/ui-react/dist'`. |
| Tailwind detecting-classes docs | Documents relative paths; `node_modules` ignored unless registered. |

**Conservative integration choice (implication, not Appica law):** use the **relative path to `dist`**, which both the local Appica dump and the package README (for older Tailwind) agree on. If adopting package-name `@source`, verify against the installed Tailwind minor before relying on it.

### 1.5 Framework notes — which stack fits embed (guaranteed + implication)

Appica documents identical package steps across frameworks; only plugin, stylesheet path, and provider placement change:

| Framework | Tailwind plugin | Global CSS (typical) | Provider wrap |
| --- | --- | --- | --- |
| Next.js App Router | `@tailwindcss/postcss` | `app/globals.css` | root layout `<body>` |
| **Vite / CRA** | **`@tailwindcss/vite`** | **`src/index.css`** | **root in `main.tsx`** |
| TanStack Start | `@tailwindcss/vite` | `src/styles/app.css` | root route |
| React Router 7 / Remix | `@tailwindcss/vite` | `app/app.css` | `root.tsx` |
| Astro | `@tailwindcss/vite` | `src/styles/global.css` | island caveats |

Local: Installation → `## Framework notes`.

**Integration implication for Skill Manager:** a **Vite client SPA** matches Appica’s documented non-RSC path and produces a static `dist/` suitable for Go `embed`. Next/Astro SSR or islands add complexity Appica does not require for a localhost embedded UI. This is an architecture implication, not an Appica mandate.

Usage also states: on a client-only stack (Vite/CRA/SPA), `'use client'` directives are a **no-op**.

### 1.6 Required root provider (guaranteed)

Wrap the app in `ThemeProvider`:

```tsx
import { ThemeProvider } from '@appica/ui-react/providers/theme-provider'
```

- Import path: `@appica/ui-react/providers/theme-provider`
- Enables theming + dark mode.
- Injects a small **inline script** that applies the stored theme **before first paint** (no flash).
- Vite/CRA: wrap the root component in `main.tsx`.
- Optional providers (only when needed):
  - `DirectionProvider` — `@appica/ui-react/providers/direction-provider` (RTL)
  - `ReducedMotionProvider` — `@appica/ui-react/providers/reduced-motion-provider`

For pure client SPAs, `suppressHydrationWarning` on `<html>` is still relevant if SSR is ever introduced; ThemeProvider’s script mutates the theme class before hydration. On a Vite SPA with no SSR, the script still runs and still uses `localStorage`.

### 1.7 Component install / import conventions (guaranteed)

Prefer **subpath imports** for smallest bundles:

```tsx
import { Button } from '@appica/ui-react/button'
import { Input } from '@appica/ui-react/input'
import { Dialog, DialogTrigger, DialogContent } from '@appica/ui-react/dialog'
```

Root barrel also works (`import { Button, Input } from '@appica/ui-react'`) but docs recommend reserving it for experiments.

Package exports (verified on `1.0.0`) include per-component subpaths, hooks under `./hooks/*`, providers under `./providers/*`, and `./styles.css`.

Icons:

```tsx
import { SunHigh, MoonStars } from '@appica/icons-react'
```

Icons default to `aria-hidden="true"` (decorative). Meaningful standalone icons need an accessible name on the control.

---

## 2. Styling, theming, and global CSS constraints

### 2.1 Token model (guaranteed)

Theming is **CSS custom properties only** — no JS theme objects.

Two layers (`# Theming`):

1. **Raw tokens** on `:root` / `.light` and `.dark` (e.g. `--primary`, `--background`, `--radius`, `--font-sans`).
2. **Theme tokens** inside Tailwind `@theme inline` (e.g. `--color-primary: var(--primary)`), which generate utilities.

Rules:

- Override **raw** tokens after the stylesheet import (`--primary`), not derived `--color-primary`.
- Utilities like `bg-primary` compile to `background-color: var(--primary)` and flip with light/dark because the raw layer changes.
- In plain CSS / arbitrary utilities, use raw tokens: `var(--primary)` or `text-(--primary)`, **not** `text-(--color-primary)`.

Dark mode is **class-based**:

```css
@custom-variant dark (&:is(.dark *));
```

When `.dark` is on an ancestor (normally `<html>`), dark tokens apply.

### 2.2 What `styles.css` actually contains (package fact)

Inspecting `@appica/ui-react@1.0.0` `styles.css`:

- Header documents consumer import: `@import '@appica/ui-react/styles.css';`
- Contains `@import 'tailwindcss';` itself, `@custom-variant dark`, reduced-motion variants (`motion-reduce` / `motion-safe`, including `[data-disable-animations]`), `@theme inline` color/radius mappings, and raw `:root` / `.dark` token values.
- Marked as the only `sideEffects` entry so bundlers keep it.

Consumers still follow the documented dual import (`tailwindcss` then `styles.css`).

### 2.3 ThemeProvider behavior that affects embedding (guaranteed)

From Theme Provider + Dark Mode + package `theme-script.js`:

| Behavior | Detail |
| --- | --- |
| Class target | Applies theme class on **`<html>`** (`document.documentElement`) |
| Default themes | `['light', 'dark']` |
| System preference | `enableSystem` default `true` → stored value may be `'system'` |
| Persistence | `localStorage` key default **`theme`** (`storageKey`) |
| No-flash script | Synchronous inline `<script>` reads storage and sets class before paint |
| `color-scheme` | `enableColorScheme` default `true` sets `documentElement.style.colorScheme` |
| CSP | `nonce` and `scriptProps` forwarded to the inline script |
| Nesting | Inner `ThemeProvider` is passthrough; only outermost owns state; nested `forcedTheme` is supported |
| Hook | `useTheme` from `@appica/ui-react/hooks/use-theme` exposes `theme`, `resolvedTheme`, `systemTheme`, `setTheme`, `mounted` |

**Integration implications:**

- Embedded UI will read/write **origin-scoped `localStorage`**. On `http://localhost:<port>` that is fine and expected. Private mode failures are caught (try/catch in script and storage hooks).
- If Skill Manager ever sets a strict **Content-Security-Policy** disallowing inline scripts, ThemeProvider’s flash-prevention script needs the documented **`nonce`** (or CSP adjustment). Appica supports nonce; it does not define a CSP.
- Forcing a single theme (e.g. always light) is supported via `forcedTheme` / `defaultTheme` / `enableSystem={false}` — product choice.

### 2.4 Fonts (guaranteed)

Defaults are **system font stacks** via `--font-sans` / `--font-mono` (no network font required). That is embed-friendly (no extra assets, no layout shift).

Custom fonts: load (Fontsource or `@font-face` on Vite), then override raw tokens after import. Appica prefers self-hosting over Google Fonts CDN.

### 2.5 Styling overrides (guaranteed)

- One-off tweaks: `className` merged with **tailwind-merge** (last conflicting utility wins).
- Composition class rule: visual overrides on the **wrapper**; structural props on the `render` JSX (matters more for RSC hydration; still the documented rule).

### 2.6 Repo rule (project constraint, not Appica)

`AGENTS.md` / wayfinder map: **all WebUI styling and components must use Appica**. Do not introduce another component library or hand-build substitutes when Appica provides the pattern. Locate components via `docs/design/llms.md`, then keyword-search `docs/design/llms-full.md`.

---

## 3. Component and client-routing behavior

### 3.1 Appica does not own the router (guaranteed)

Appica is UI primitives + styled compounds. Routing is **your** framework/router. Composition docs exist specifically to project Appica parts onto router links without losing styles/behavior.

### 3.2 Composition patterns (guaranteed)

1. **`render` prop** (from Base UI): adopt another element/component while keeping Appica styling/behavior.

```tsx
import Link from 'next/link' // or react-router Link
import { NavigationLink } from '@appica/ui-react/navigation'

<NavigationLink render={<Link href="/pricing" />}>Pricing</NavigationLink>
```

2. **Do not** `render` an `<a>` through `Button` — Button enforces button semantics. For a link that *looks* like a button, use **`buttonVariants`**:

```tsx
import { buttonVariants } from '@appica/ui-react/button'

<a href="/docs" className={buttonVariants({ variant: 'primary', size: 'md' })}>
  Read the docs
</a>
```

Also exported: `inputVariants`, `navigationLinkVariants`.

### 3.3 Navigation component (guaranteed)

`@appica/ui-react/navigation`:

- Landmark `<nav>`; give it `aria-label`.
- `NavigationLink` defaults to `<a>`; mark current via `active` or root `activeLink` + per-link `value` (`aria-current="page"`).
- Project onto a client router with `render`.
- Use Navigation for **distinct pages/sections**; Tabs for in-page panels; Breadcrumb for trail; Navigation Menu for rich hover panels.

### 3.4 Client-side routing implications for chi embed (integration only)

Appica is silent on history strategy. For a static SPA served by chi:

| Topic | Implication | Owner |
| --- | --- | --- |
| History API (`BrowserRouter` / data router) | Deep links like `/skills/foo` require the **server** to serve `index.html` for non-file routes (SPA fallback). Otherwise refresh/deep-link 404s. | Go/chi static middleware |
| Hash routing | Avoids server fallback (`/#/skills/foo`) but changes URL UX; Appica neither requires nor forbids it. | Product choice |
| Basename / mount path | If UI is not at `/`, client router basename, Vite `base`, and chi mount must agree. | Bundler + router + chi |
| Appica links | Default `<a href>` does full navigation unless composed onto router `Link` via `render` or variant-styled router links. | WebUI code |
| API vs UI routes | REST JSON endpoints must be registered **outside** or **before** the SPA catch-all so `/api/...` is never rewritten to `index.html`. | chi router layout |

Appica’s documented Vite path does not set a router; Skill Manager must pick one and compose Navigation accordingly.

---

## 4. Build output and asset base path

### 4.1 What Appica guarantees

**Nothing about production bundle layout, hashing, `base` href, or embed.** Appica assumes a normal React app build. Framework notes stop at Tailwind plugin + provider placement.

Therefore build/embed facts come from the **chosen bundler** and Go.

### 4.2 Vite defaults relevant to embed (integration; Vite docs)

Assuming the Appica-documented Vite client stack:

| Option | Default | Relevance |
| --- | --- | --- |
| `build.outDir` | `dist` | Directory to `//go:embed` |
| `build.assetsDir` | `assets` | JS/CSS/image chunk subdirectory under `outDir` |
| `base` | `/` | Prefix for all built asset URLs and `import.meta.env.BASE_URL` |
| Relative base | `base: './'` or `''` | Vite documents this for **embedded deployment** when base path is unknown |
| `publicDir` | `public` | Copied as-is to `outDir` root |
| Entry | root `index.html` | SPA shell chi must serve |

Public base path (https://vite.dev/guide/build.html#public-base-path):

- Nested deploy path → set `base` (e.g. `/ui/`); JS-imported assets, CSS `url()`, and HTML references are rewritten at build.
- Dynamic URL concatenation must use `import.meta.env.BASE_URL` exactly (statically replaced).
- Relative base (`./` or `''`) makes generated URLs relative to each file — Vite explicitly calls out **embedded deployment**.

Static hosting guides show SPA **rewrite-to-`index.html`** patterns (e.g. Firebase `"rewrites": [{ "source": "**", "destination": "/index.html" }]`). Chi needs the equivalent.

### 4.3 Go `embed` + chi serving implications (integration only)

Not Appica; required by Skill Manager’s preferred “one executable” shape:

1. **Build order:** Node build of the WebUI → output directory present before `go build` so `//go:embed` sees files (embed is compile-time).
2. **`embed.FS`:** embed the Vite `dist` tree (or a staged copy). Prefer embedding the directory contents in a way that preserves `index.html` at the FS root you serve.
3. **`http.FileServer` / chi:** serve embedded files with correct URL stripping when mounted under a prefix (`http.StripPrefix`).
4. **SPA fallback:** for `BrowserHistory`, non-file GETs under the UI mount should return `index.html` (not 404), while real files (`/assets/*`) are served as-is.
5. **MIME types:** Go’s `FileServer` sets Content-Type from extension; hashed `.js`/`.css` are fine. Ensure `.svg`/fonts if self-hosted fonts are added later.
6. **Caching implication:** hashed assets under `assets/` are cache-friendly; `index.html` should not be long-cached if future in-place binary updates are expected (Vite’s preload-error guidance). Product/ops choice.
7. **Path alignment checklist:**

| Layer | Must agree |
| --- | --- |
| Vite `base` | Asset URLs baked into `index.html` and chunks |
| Router `basename` | Client route matching |
| chi mount path | Where `FileServer` and SPA fallback are attached |
| REST prefix | Distinct from UI asset routes |

**Relative Vite `base` (`./`)** reduces mount-path coupling for asset URLs; router basename and chi mount may still need explicit configuration if the UI is not at `/`.

### 4.4 What the built artifact is *not*

- Not an Appica-specific bundle format.
- Not SSR HTML from Appica components (unless Skill Manager chooses Next/other SSR — unnecessary for local embed).
- Not a pre-themed CSS file independent of the app build — **Tailwind compilation is part of the WebUI build**, and that compiled CSS ships inside the Vite output that Go embeds.

---

## 5. Accessibility constraints relevant to an embedded WebUI

### 5.1 Provided by Appica (guaranteed)

From Accessibility + Animation + Reduced Motion Provider:

- WAI-ARIA patterns via Base UI: keyboard interaction, focus management, ARIA wiring.
- Overlays (Dialog, Drawer, Popover) trap focus and restore to trigger.
- Global **`:focus-visible`** focus ring using design tokens — do not strip outlines.
- **Reduced motion:** animations respect `prefers-reduced-motion`; `ReducedMotionProvider` can force-disable via `data-disable-animations` on `<html>` (needed because portaled popups escape React subtrees).
- Components tested with `vitest-axe`; composition can still introduce issues.

### 5.2 Skill Manager / app responsibilities (guaranteed as “yours to handle”)

- Visible labels via `Field` / `FieldLabel` for single controls.
- Groups (`RadioGroup`, `CheckboxGroup`, `ToggleGroup`): use `aria-label` / `aria-labelledby`, not `FieldLabel`.
- Icon-only controls: `aria-label` (e.g. theme toggle pattern in Dark Mode docs).
- Recolor → re-check contrast (WCAG), especially `*-foreground` on fills.
- RTL: use `DirectionProvider` if shipping RTL.
- Audit assembled pages (axe/Lighthouse) and keyboard-only flows.
- Icons package: decorative by default (`aria-hidden`); name meaningful icons/controls.

### 5.3 Embed-specific a11y notes (integration)

- Localhost-only does **not** relax a11y obligations Appica documents.
- Theme toggle sample already shows the `mounted` guard + `aria-label="Toggle theme"` pattern.
- If animations are product-toggled, use `ReducedMotionProvider` rather than stripping CSS globally in a way that fights tokens.

---

## 6. Minimal embed-oriented setup sketch

Illustrative only — not a product decision and not an Appica-prescribed repo layout.

**WebUI (Vite + React 19 + Tailwind 4 + Appica):**

```text
webui/
  package.json          # react@19, react-dom@19, @appica/ui-react, tailwindcss, @tailwindcss/vite, vite
  index.html
  vite.config.ts        # tailwind plugin; base: '/' or './' or '/ui/'
  src/main.tsx          # ThemeProvider wrap; router root
  src/index.css         # @import tailwindcss; @import styles.css; @source .../dist
  src/App.tsx           # routes + Appica components via subpath imports
```

**Go:**

```text
//go:embed all:webui/dist
var webuiFS embed.FS

// chi:
//  - /api/*  → REST JSON (Skill Manager core)
//  - /*      → static from webuiFS + SPA index fallback
```

Build pipeline implication: `npm run build` in `webui/` before `go build`.

---

## 7. Fact vs implication matrix

| Topic | Appica-guaranteed | Integration implication for Go/chi embed |
| --- | --- | --- |
| React 19 + Tailwind 4 | Yes | WebUI toolchain must include them |
| `@appica/ui-react` + `styles.css` + `@source` | Yes | Run Tailwind at **WebUI build time**; embed compiled CSS, not raw Appica package |
| `ThemeProvider` | Yes | Inline script + `localStorage`; consider CSP nonce if CSP added |
| Subpath imports / `render` composition | Yes | Compose onto chosen router |
| Router choice | No | Product: Browser vs Hash; basename |
| Vite `base` / `outDir` | No (Vite) | Align with chi mount; relative base option for embed |
| SPA fallback | No (host) | Required for History API deep links |
| `embed.FS` single binary | No (Go) | Preferred Skill Manager architecture per wayfinder map |
| REST layout | No | Keep API routes out of SPA fallback |
| Icons package | Optional companion | Use when UI needs icons; a11y defaults apply |
| Forced single theme | Supported API | Product whether to follow OS dark mode |

---

## 8. Gaps and open decisions

These are **not** answered by Appica and should not be invented here:

1. **UI mount path** — `/` vs `/ui` vs other; drives Vite `base`, router basename, chi mount.
2. **History vs hash routing** — server fallback complexity vs URL shape.
3. **Theme product policy** — follow system, default light, or forced theme; storage key namespace if multiple local apps share origin (unlikely on distinct ports).
4. **CSP** — whether the local server sends CSP that blocks inline theme script.
5. **Icons / flags packages** — adopt `@appica/icons-react` (examples assume it) or subset.
6. **Custom brand tokens** — whether Skill Manager recolors Appica or keeps defaults.
7. **RTL / i18n** — out of current MVP language unless added later.
8. **Exact `@source` form vs Tailwind minor** — resolve against pinned Tailwind at implementation time (see discrepancy in §1.4).
9. **Dev mode** — Vite dev server proxy to chi vs chi-only serving of built assets; Appica silent.
10. **Package churn** — only `1.0.0` observed; re-verify peers/exports on upgrade.

---

## 9. Direct answers to the ticket question

| Concern | Answer |
| --- | --- |
| **Exact project setup** | React 19 app + Tailwind v4 pipeline + `@appica/ui-react` (+ optional `@appica/icons-react`). Import `styles.css`, add `@source` to package `dist`, wrap with `ThemeProvider`. Vite is the documented client SPA fit. Node ≥ 20 for installing the package as published. |
| **Styling / component constraints** | No second UI kit. Token overrides via CSS variables after import. Class-based dark mode on `<html>`. Subpath imports. Compose with `render` / variant helpers. Prefer Appica components from the catalog for UI chrome. |
| **Build output** | Appica does not define it. With Vite: static `dist/` (`index.html` + `assets/*` hashed bundles) including **Tailwind-compiled** CSS. That tree is what Go should embed. |
| **Asset base behavior** | Appica-unspecified. Vite `base` controls baked asset URLs; `base: './'` is Vite’s embed-oriented option. Must align with how chi serves the FS. |
| **Client-side routing** | Appica-unspecified beyond composition onto your router. History API ⇒ chi SPA fallback; Hash API ⇒ no fallback. Always compose `NavigationLink`/links onto the real router and keep API routes off the fallback. |
| **Accessibility** | Keep Appica’s focus rings, labeling patterns, reduced motion, and icon a11y defaults; app still owns labels, contrast after recolor, and page-level audits. |

---

## 10. Citation index (material claims)

| Claim | Citation |
| --- | --- |
| React ≥ 19 hard requirement; Tailwind ≥ 4; no prebuilt component CSS; `@source` required | https://appica.dev/ui/docs/react/installation — Prerequisites, Configure Tailwind, Why there's no prebuilt CSS; local `llms-full.md` `# Installation` |
| Framework table (Vite plugin + `src/index.css` + `main.tsx` provider) | Installation → Framework notes |
| Subpath imports; client SPA `'use client'` no-op | https://appica.dev/ui/docs/react/usage — Importing components, Server Components |
| `render` prop; don’t fake buttons as links; variant helpers | https://appica.dev/ui/docs/react/composition |
| Raw vs theme tokens; class-based dark | https://appica.dev/ui/docs/react/theming , `/dark-mode` |
| ThemeProvider props, flash script, `storageKey`, nonce | https://appica.dev/ui/docs/react/theme-provider ; package `theme-script.js` |
| A11y handled vs app-owned | https://appica.dev/ui/docs/react/accessibility |
| System fonts default | https://appica.dev/ui/docs/react/fonts |
| Navigation + router projection | https://appica.dev/ui/components/react/navigation |
| Package peers/exports/engines/sideEffects | npm `@appica/ui-react@1.0.0` metadata and tarball |
| `@source` relative path / node_modules ignored | https://tailwindcss.com/docs/detecting-classes-in-source-files |
| Vite `base`, relative base for embed, `outDir`/`assetsDir` | https://vite.dev/guide/build.html , `/config/shared-options.html`, `/config/build-options.html` |
| SPA rewrite examples | https://vite.dev/guide/static-deploy.html |
| Go embed is compile-time file inclusion | https://pkg.go.dev/embed |

---

## 11. Bottom line for implementers

Skill Manager’s embedded WebUI is **“a Vite-built React 19 SPA that depends on Appica + Tailwind 4, with its `dist/` embedded and served by chi.”**  
Appica constrains the **React/Tailwind/component** side tightly.  
It leaves the **binary embed, URL layout, and SPA fallback** to Go/chi and the bundler — those must be designed explicitly, with path alignment as the main integration hazard.
