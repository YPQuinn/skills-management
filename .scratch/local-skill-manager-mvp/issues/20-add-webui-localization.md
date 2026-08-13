# Add English and Simplified Chinese WebUI localization

Type: task
Status: resolved
Blocked by: 12

## Goal

Add a small, maintainable localization seam to the existing Appica WebUI without starting Ticket 13. Operators can switch between English and Simplified Chinese, and the choice persists locally.

## Scope

- Localize all existing WebUI-owned visible copy, accessible names, placeholders, statuses, empty states, dialogs, validation messages, and client-side fallback errors.
- Support `en` and `zh-CN`.
- Default from a persisted choice, then the browser language (`zh*` selects `zh-CN`; otherwise English).
- Add an Appica language Select beside the existing theme Select.
- Persist the choice in local storage and synchronize `document.documentElement.lang` and the document title.
- Format WebUI-owned timestamps with the selected locale.
- Keep English behavior byte-compatible where current tests depend on exact copy.
- Add behavior tests for initial locale selection, switching, persistence, HTML language, and representative translated pages/actions.
- Keep all changed/new TS and TSX files at or below 250 pure LOC and rebuild the embedded `dist` output.

## Boundaries

- Ticket 13 Groups/Targets/Assignments behavior is not started. Existing navigation and placeholder copy may be translated only.
- CLI help/output and REST/core error messages remain English in this task; JSON contracts are unchanged.
- Server-provided free-text messages (`error.message`, Source issue reasons, and stored last errors) are displayed verbatim rather than guessed or translated client-side.
- Do not add an i18n dependency for two locales; use a typed local dictionary, React context, and platform `Intl`/storage APIs.
- Continue using Appica components and exact documented APIs.

## Acceptance

- Switching language updates the current screen immediately without navigation or reload.
- A reload restores the chosen language.
- Browser `zh`, `zh-CN`, and `zh-TW` select Simplified Chinese only when no explicit choice exists; all other browser languages select English.
- `<html lang>` matches the active locale.
- English and Simplified Chinese cover the current Setup, global navigation, Skills, Sources, import/replace, status, error, and not-found UI.
- Dynamic resource names, slugs, paths, and server-provided messages are preserved exactly.
- Existing English behavior tests remain valid, and focused localization tests cover both locales.
- WebUI lint, tests, and build pass; Go tests remain green; no staged files are created.

## Answer

Implemented a dependency-free, typed WebUI localization layer for English and Simplified Chinese. The Appica header now exposes a persisted language Select; browser-language initialization, immediate switching, `<html lang>`, document title, timestamps, visible copy, accessible names, placeholders, statuses, dialogs, and client-owned fallback errors all follow the active locale. Server-provided messages and domain values remain verbatim, and locale changes do not refetch or blank current resources.

Added focused behavior, accessibility, error-boundary, persistence, interpolation, and no-refetch tests. Final validation passed with 56 WebUI tests, lint, production build, full Go tests, vet, diff checks, pure-LOC limits, an empty staged index, and independent xhigh acceptance review.
