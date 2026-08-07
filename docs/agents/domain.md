# Domain Docs

This repository uses a single-context domain documentation layout.

## Before exploring, read these

- `docs/CONTEXT.md` for the domain glossary
- Relevant ADRs under `docs/adr/`

If a file or directory does not exist, proceed silently. Create domain documentation lazily when terminology or a durable architectural decision is resolved.

## Use the glossary's vocabulary

Use the canonical terms defined in `docs/CONTEXT.md` in tickets, specifications, code, tests, and user-facing text. Do not drift to synonyms listed under `_Avoid_`.

## Flag ADR conflicts

If proposed work contradicts an existing ADR, surface the conflict explicitly rather than silently overriding it.
