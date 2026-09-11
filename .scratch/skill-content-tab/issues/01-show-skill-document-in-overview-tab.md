# Show the Skill document in the Skill detail Overview tab

Type: task
Status: resolved

## Question

The Skill detail Overview tab shows slug, timestamps, Store/Baseline digests, and the Source Binding block. The page header already carries slug, name, description, and the bound/unbound badge, and the Synchronization tab already carries the digests, so the tab tells the operator nothing they need. Show the Skill's own `SKILL.md` instead: its YAML frontmatter parsed into separate fields, and its Markdown body rendered.

## Comments

- The tab keeps its name (Overview / 概览) and its `?tab=` value (`overview`); only its content changes.
- `SKILL.md` is not exposed by any API today. The database holds only `name`, `description`, and digests; the document lives on the filesystem at `<StoreRoot>/<slug>/SKILL.md`. A read-only `GET /api/v1/skills/{id}/content` endpoint is required.
- The YAML frontmatter is parsed on the backend with the existing `gopkg.in/yaml.v3`, preserving document order, so the WebUI needs no YAML parser and folded scalars (`description: >-`) survive.
- The WebUI renders the Markdown body with `react-markdown` + `remark-gfm`; Appica provides no Markdown or prose component.
- Scope is `SKILL.md` only: no directory listing, no editing, no CLI change.

## Answer

The Overview tab now renders the Skill's `SKILL.md`: its frontmatter fields as an ordered key/value list, then its Markdown body. The tab name and `?tab=` value are unchanged; the digests and the Source Binding block are gone, since the header and the Synchronization tab already carry them.

- `source.ParseSkillDocument` splits one `SKILL.md` into ordered `SkillField`s and its body, walking a `yaml.Node` mapping so document order survives and folded scalars collapse to one string. A non-scalar value keeps its YAML text. A missing frontmatter block is not an error; a present but unparsable one is.
- `App.SkillContent` reads the live Store copy at `<StoreRoot>/<slug>/SKILL.md`. A missing Skill or missing file is `CodeNotFound`; invalid frontmatter is `CodeConflict`, so a hand-broken document sends the operator to the Synchronization tab instead of being silently patched over.
- `GET /api/v1/skills/{id}/content` returns `skill_id`, `slug`, `path`, `frontmatter` (always an array), and `body`.
- The WebUI renders the body with `react-markdown` + `remark-gfm`, mapping elements onto Appica tokens and shifting body headings down one level so the page keeps a single `h1`.

Three browser assertions had to follow the new tab content, and only the Playwright suite caught them:

- The Skill fixture's body is `# <name>`, so its rendered heading now shares the Skill name with the page `h1`. `getByRole('heading', { name: <slug> })` became a strict-mode violation in `a11y.spec.ts`, `responsive-theme.spec.ts`, and `chromium-journey.spec.ts`; each now pins `level: 1`, which also guards the single-`h1` contract.
- `chromium-journey.spec.ts` asserted the deleted `unboundSkillText` string. The recovered Skill's unbound state is now proven by its `h1` plus the Synchronization tab's `Unbound Skill` Alert.

A worktree built from `aaaabcf` fails the same three specs for the same pre-existing reasons (one Tabs-trigger contrast violation, two toast-versus-dialog locator collisions), so this change adds no e2e failure.
