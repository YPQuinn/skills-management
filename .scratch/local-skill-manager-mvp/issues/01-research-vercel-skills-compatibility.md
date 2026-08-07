# Research Vercel Skills compatibility surface

Type: research
Status: resolved
Blocked by:

## Question

What source locators, repository layouts, Skill selection rules, supported Agents and scopes, target directories, installation behavior, and version metadata does the current `vercel-labs/skills` project officially support?

## Answer

Full brief: [`docs/research/vercel-skills-compatibility.md`](../../../docs/research/vercel-skills-compatibility.md)

Inspected `vercel-labs/skills@a4d243c` (`skills@1.5.22`, 2026-08-05) on 2026-08-07.

**Headline facts:**
- Source types: `local`, `github`, `gitlab`, `git`, `well-known`, `download` — covering owner/repo shorthand, tree/subpath/`@skill`/`#ref`, SSH/HTTPS git, well-known agent-skills indexes, and direct SKILL.md/archive URLs.
- Discovery: priority containers (incl. `skills/` + agent dirs + Claude plugin manifests), depth-3 nested catalogs, root SKILL.md short-circuit, `--full-depth` recursive fallback; requires frontmatter `name` + `description`.
- 76 `--agent` keys with asymmetric project/global paths; canonical store is `.agents/skills` (project) / `~/.agents/skills` (global); symlink-to-canonical or `--copy`.
- Dual locks: global `~/.agents/.skill-lock.json` (v3, tree SHA) and project `skills-lock.json` (v1, content hash); `update` reinstalls on hash/digest change.
- Auth is ambient only (git/`gh`/env tokens); CLI overwrites existing skill destinations after confirm.

Implications for Skill Manager and residual upstream gaps are separated in the brief; no product decisions made.
