# Research: Vercel Skills compatibility surface

**Question:** What source locators, repository layouts, Skill selection rules, supported Agents and scopes, target directories, installation behavior, and version metadata does the current `vercel-labs/skills` project officially support?

**Inspected artifact:** [vercel-labs/skills](https://github.com/vercel-labs/skills)  
**Inspected commit:** [`a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5`](https://github.com/vercel-labs/skills/commit/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5) (tag/release `v1.5.22`, 2026-08-05)  
**Package:** npm `skills@1.5.22` (`bin`: `skills`, `add-skill`)  
**Inspected date:** 2026-08-07  
**Primary sources:** repository README, TypeScript sources under `src/`, and package metadata at that commit. No secondary write-ups were used as authority.

---

## 1. Facts (what upstream does)

### 1.1 Product identity

- The project is a Node CLI for discovering and installing Agent Skills into local coding-agent skill directories.
- Package name: `skills` ([package.json](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/package.json)).
- Requires Node `>=22.20.0`.
- Entry: `npx skills …` / `npx add-skill …`.
- Core commands: `add`, `use`, `list`/`ls`, `find`, `remove`/`rm`, `update`/`upgrade`/`check`, `init`, plus experimental `experimental_install` and `experimental_sync` ([cli.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/cli.ts)).

### 1.2 Source locator forms and source types

`parseSource()` returns one of six types ([types.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/types.ts#L140-L147), [source-parser.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts)):

| `ParsedSource.type` | Accepted locator forms | Notes |
|---|---|---|
| `local` | Absolute path; `./…`; `../…`; `.`; `..`; Windows drive paths (`C:\…`) | Resolved with `path.resolve`. No existence check in the parser. |
| `github` | `owner/repo`; `owner/repo/subpath`; `owner/repo@skill-name`; `github:owner/repo…`; `https://github.com/owner/repo`; tree URLs with branch and optional subpath | Public github.com fast path. `GH_HOST` other than `github.com` forces shorthand into generic `git` instead. |
| `gitlab` | `gitlab:owner/repo…`; `https://gitlab.com/…`; any host’s `/-/tree/<ref>[/<subpath>]` GitLab URL shape | Subgroups supported via non-greedy path match. |
| `git` | Any remaining git-like URL (SSH `git@host:…`, `ssh://…`, other HTTPS `.git`, GHE hosts, fallback) | Generic clone path. |
| `well-known` | Arbitrary `http(s)://` URL that is not a known git host and does not end in `.git` | Discovery via `/.well-known/agent-skills/index.json` then `/.well-known/skills/index.json`. |
| `download` | Hosted artifact URLs (raw/codeload/objects.githubusercontent.com; github.com `archive/`/`raw/`/`releases/download/`; gitlab.com `/-/archive|raw/`) | Direct SKILL.md or archive download; not normalized to a parent repo clone. |

Additional locator details:

- **Fragment refs:** `#<ref>` and `#<ref>@<skill>` on git-like sources become `ref` / `skillFilter` ([source-parser.ts L204–231](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts#L204-L231)).
- **`@skill` shorthand:** `owner/repo@skill-name` sets `skillFilter` without a path ([L450–459](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts#L450-L459)).
- **Subpath sanitization:** any `..` segment in a subpath throws ([L106–122](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts#L106-L122)).
- **Aliases:** `coinbase/agentWallet` → `coinbase/agentic-wallet-skills`; `vercel-labs/vercel-skills` → `vercel-labs/agent-skills` ([L145–148](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts#L145-L148)).
- **GitHub Enterprise:** `GH_HOST` hostname only; invalid values fall back to `github.com` ([github-host.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/github-host.ts)). GHE sources use generic `git` cloning, not the GitHub.com Trees/blob APIs.
- **Well-known then download:** for `type: 'well-known'`, `add` first tries well-known discovery; on failure it falls back to direct download of the same URL ([add.ts L1128–1134, L1167–1176](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1128-L1176)).

README-documented source examples match the parser: GitHub shorthand/URL/tree path, GitLab URL, any git URL, local path, and direct SKILL.md/archive URL ([README Source Formats](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/README.md)).

### 1.3 How sources are fetched

| Source type | Fetch method |
|---|---|
| `local` | Read path in place |
| `github` (default depth) | Prefer **blob fast path** for allowlisted owners/repos; else shallow git clone |
| `github` with `--full-depth`, `gitlab`, `git` | Always shallow git clone (`--depth 1`, optional `--branch <ref>`) |
| `well-known` | HTTP fetch of discovery index + skill artifacts |
| `download` / well-known fallback | HTTP download of SKILL.md or archive |

**Git clone** ([git.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/git.ts)):

- Default timeout 300s; override `SKILLS_CLONE_TIMEOUT_MS`.
- Allowed protocols: `https`, `http`, `ssh`, `git`, `file`. `ext::` rejected.
- `GIT_TERMINAL_PROMPT=0` (non-interactive).
- LFS filters disabled; LFS smudge skipped.
- On GitHub HTTPS auth failure: try `gh repo clone`, then SSH `BatchMode=yes` fallback.
- Temp clone under OS temp dir; cleaned after install.

**Blob / skills.sh fast path** ([blob.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/blob.ts), [add.ts L1178–1202](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1178-L1202)):

1. GitHub Trees API recursive tree.
2. Discover `SKILL.md` paths in tree.
3. Fetch frontmatter via `raw.githubusercontent.com`.
4. Download full skill snapshots from `https://skills.sh/api/download/<owner>/<repo>/<slug>` (or self-hosted map entry).
- Owner allowlist: `vercel`, `vercel-labs`, `heygen-com`.
- Repo allowlist map currently includes `zapier/connectors` (self-hosted download URL).
- Disabled when `--full-depth` is set.
- Any partial blob failure falls back to full clone.
- Root-level blob skills install **only** `SKILL.md` from the snapshot (supporting files stripped) ([blob.ts L560–572](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/blob.ts#L560-L572)). Disk/clone installs of root skills still copy the full skill directory excluding `.git` etc.

**Direct download** ([download-source.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/download-source.ts)):

- Accepts valid SKILL.md (name+description frontmatter) or zip / tar / tar.gz / tgz (magic-byte detection; extension optional).
- Defaults: download ≤ 10 MiB, extract ≤ 25 MiB, ≤ 1000 files.
- Overrides: `SKILLS_DOWNLOAD_MAX_BYTES`, `SKILLS_EXTRACT_MAX_BYTES`, `SKILLS_EXTRACT_MAX_FILES`.
- 30s fetch timeout; follows redirects.
- Single top-level archive directory is unwrapped (ignores `__MACOSX`).

**Well-known discovery** ([providers/wellknown.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/providers/wellknown.ts)):

- Probe order per base URL: path-relative then host-root for:
  - `/.well-known/agent-skills/index.json` (preferred)
  - `/.well-known/skills/index.json` (legacy)
- **v0.2.0** index: `$schema: https://schemas.agentskills.io/discovery/0.2.0/schema.json`; entries require `name`, `type` (`skill-md`|`archive`), `description`, `url`, `digest` (`sha256:<64 hex>`). Digest verified before install.
- **Legacy v0.1.0** (no `$schema`): entries require `name`, `description`, `files[]` including `SKILL.md`; files fetched individually from the well-known skill directory.
- Skill names in index must match `^[a-z0-9-]+$` (1–64 chars, no leading/trailing `-`, no `--`).
- Unknown `$schema` values are rejected.

### 1.4 Repository and nested Skill discovery

Discovery is implemented by `discoverSkills()` ([skills.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts)).

**Valid Skill definition:**

- Directory containing `SKILL.md`.
- YAML frontmatter must include string `name` and string `description`.
- Optional `metadata.internal: true` hides the skill unless `INSTALL_INTERNAL_SKILLS=1|true` or the user explicitly requested skills by name (`--skill` / `@skill`) ([skills.ts L58–61, L115–120](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts#L58-L120)).

**Search order / rules:**

1. If `searchPath` itself has `SKILL.md` and it is not an already-installed project skill tracked in `skills-lock.json`, parse it and **return early** unless `--full-depth`.
2. Walk **priority containers** (depth rules below):
   - repo/`searchPath` root (depth **1** only — immediate children)
   - `skills/`
   - `skills/.curated/`, `skills/.experimental/`, `skills/.system/`
   - agent project skill dirs (hardcoded list; see below)
   - paths declared by Claude plugin manifests
3. Known skill **containers** (everything after root) walk up to `DEFAULT_SKILL_CONTAINER_DEPTH = 3` levels ([constants.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/constants.ts)).
4. A `SKILL.md` found at a shallower level **stops descent** under that directory (shadowing).
5. Plugin-manifest-declared directories stay at depth 1.
6. If nothing found, or `--full-depth`, recursive walk up to depth 5 (`findSkillDirs`).
7. Duplicate skill **names** are skipped after the first (`seenNames`).
8. Already-installed project skills under agent dirs that appear in `skills-lock.json` are skipped during discovery (avoids rediscovering distributed copies as sources).
9. Skip directory names: `node_modules`, `.git`, `dist`, `build`, `__pycache__`.

**Agent project skill dirs used during discovery** ([skills.ts L12–40](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts#L12-L40)):

```
.agents/skills, .claude/skills, .cline/skills, .codebuddy/skills,
.codex/skills, .commandcode/skills, .continue/skills, .github/skills,
.goose/skills, .grok/skills, .iflow/skills, .junie/skills,
.kilocode/skills, .kimchi/skills, .kiro/skills, .minimax/skills,
.mux/skills, .neovate/skills, .opencode/skills, .openhands/skills,
.pi/skills, .qoder/skills, .roo/skills, .trae/skills, .windsurf/skills,
.zcode/skills, .zencoder/skills
```

This discovery list is **not identical** to install target paths (e.g. install for Codex/OpenCode/Cursor uses `.agents/skills`, while discovery also looks at `.codex/skills` / `.opencode/skills` / `.github/skills`).

**Plugin manifest discovery** ([plugin-manifest.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/plugin-manifest.ts)):

- Reads `.claude-plugin/marketplace.json` and/or `.claude-plugin/plugin.json`.
- Only local string `source` values starting with `./`; remote plugin sources skipped.
- `pluginRoot` and skill paths must start with `./`.
- Declared skill paths are not subject to the depth-3 catalog walk; parent dirs are searched at depth 1.
- Always also adds each plugin’s conventional `skills/` directory.
- Plugin `name` is attached as `skill.pluginName` for grouping in UI.

**Subpath constraint:** if the locator includes a subpath, discovery is restricted to that subtree; path-traversal outside the repo root is rejected ([skills.ts L166–184](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts#L166-L184)).

### 1.5 Multi-Skill selection behavior

During `skills add` ([add.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts)):

| Condition | Selection |
|---|---|
| `--skill '*'` or `--all` | All discovered skills |
| `--skill <names…>` and/or `@skill` / `#ref@skill` filter | Case-insensitive exact match on frontmatter `name` or display name ([filterSkills](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts#L308-L318)); no match → error + list available |
| Exactly one skill discovered | Auto-select |
| `--yes` and multiple skills | Install all |
| Interactive TTY, multiple skills | Multiselect UI; groups by plugin name when present |
| `--list` | List only; no install |

`--all` expands to `--skill '*' --agent '*' -y` ([add.ts L1066–1070](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1066-L1070)).

When any skill names are explicitly requested, internal skills become visible (`includeInternal`).

### 1.6 Supported Agent names

At the inspected commit there are **76** `AgentType` values ([types.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/types.ts), [agents.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/agents.ts)).

CLI `--agent` values (exact keys):

`aider-desk`, `amp`, `antigravity`, `antigravity-cli`, `astrbot`, `autohand-code`, `augment`, `bob`, `claude-code`, `openclaw`, `cline`, `codearts-agent`, `codebuddy`, `codemaker`, `codestudio`, `codex`, `command-code`, `continue`, `cortex`, `crush`, `cursor`, `deepagents`, `devin`, `dexto`, `droid`, `eve`, `firebender`, `forgecode`, `gemini-cli`, `github-copilot`, `goose`, `grok`, `hermes-agent`, `inference-sh`, `iflow-cli`, `jazz`, `junie`, `kilo`, `kimchi`, `kimi-code-cli`, `kiro-cli`, `kode`, `lingma`, `loaf`, `mcpjam`, `minimax-code`, `mistral-vibe`, `moxby`, `mux`, `neovate`, `opencode`, `openhands`, `ona`, `pi`, `qoder`, `qoder-cn`, `qwen-code`, `replit`, `reasonix`, `roo`, `rovodev`, `tabnine-cli`, `terramind`, `tinycloud`, `trae`, `trae-cn`, `warp`, `windsurf`, `zed`, `zcode`, `zencoder`, `zenflow`, `pochi`, `promptscript`, `adal`, `universal`.

Invalid `--agent` values error with the full valid list.

**Universal agents:** any agent whose project `skillsDir === '.agents/skills'` and `showInUniversalList !== false`. These share the canonical directory and do not need per-agent symlinks for that path ([agents.ts L686–712](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/agents.ts#L686-L712)). Examples include `amp`, `cline`, `codex`, `cursor`, `gemini-cli`, `github-copilot`, `opencode`, `replit` (list-hidden), `universal` (list-hidden), etc.

**Project-only agents** (`globalSkillsDir: undefined`): `eve`, `promptscript`.

**Special cases:**

- **Eve:** project path `agent/skills/`; optional subagents at `agent/subagents/<name>/skills` via `--subagent` (`root`/`.` = root agent). Eve SKILL.md frontmatter is stripped to a reduced set on install.
- **OpenClaw global:** prefers `~/.openclaw/skills`, else legacy `~/.clawdbot/skills` or `~/.moltbot/skills`.
- **Env overrides:** `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `VIBE_HOME`, `HERMES_HOME`, `AUTOHAND_HOME`, `GROK_HOME`; XDG config home via `xdg-basedir`.
- **Detection:** each agent has a `detectInstalled()` heuristic (presence of config dirs / apps). If none detected and `--yes`, install targets **all** agents; otherwise interactive selection. When agents are auto-selected from detection (single agent or `--yes` with detections), universal agents are also added via `ensureUniversalAgents`.

### 1.7 Scopes and exact target paths

**Scopes:**

| Scope | Flag | Canonical content store | Default |
|---|---|---|---|
| Project | (default) | `<cwd>/.agents/skills/<skill>/` | Yes |
| Global | `-g` / `--global` | `~/.agents/skills/<skill>/` | No |

Interactive installs without `-y` prompt for scope when any selected agent supports global install.

**Canonical vs agent path** ([installer.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/installer.ts)):

- Canonical always: `{cwd|home}/.agents/skills/<sanitized-name>/` (except Eve symlink mode, which uses Eve’s own base).
- Agent install path: `getAgentBaseDir(agent, global)` + `/<sanitized-name>/`.
- Skill directory name = `sanitizeName(skill.name)`: lowercased; non `[a-z0-9._]` → `-`; trim leading/trailing `.`/`-`; max 255; empty → `unnamed-skill`.

**Exact project / global paths per agent** (from [agents.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/agents.ts); `~` = home, `$XDG_CONFIG_HOME` falls back to `~/.config`):

| `--agent` | Project path | Global path |
|---|---|---|
| `aider-desk` | `.aider-desk/skills/` | `~/.aider-desk/skills/` |
| `amp` | `.agents/skills/` | `$XDG_CONFIG_HOME/agents/skills/` |
| `antigravity` | `.agents/skills/` | `~/.gemini/antigravity/skills/` |
| `antigravity-cli` | `.agents/skills/` | `~/.gemini/antigravity-cli/skills/` |
| `astrbot` | `data/skills/` | `~/.astrbot/data/skills/` |
| `autohand-code` | `.autohand/skills/` | `$AUTOHAND_HOME/skills` (default `~/.autohand/skills`) |
| `augment` | `.augment/skills/` | `~/.augment/skills/` |
| `bob` | `.bob/skills/` | `~/.bob/skills/` |
| `claude-code` | `.claude/skills/` | `$CLAUDE_CONFIG_DIR/skills` (default `~/.claude/skills`) |
| `openclaw` | `skills/` | `~/.openclaw/skills/` (or clawdbot/moltbot legacy) |
| `cline` | `.agents/skills/` | `~/.agents/skills/` |
| `codearts-agent` | `.codeartsdoer/skills/` | `~/.codeartsdoer/skills/` |
| `codebuddy` | `.codebuddy/skills/` | `~/.codebuddy/skills/` |
| `codemaker` | `.codemaker/skills/` | `~/.codemaker/skills/` |
| `codestudio` | `.codestudio/skills/` | `~/.codestudio/skills/` |
| `codex` | `.agents/skills/` | `$CODEX_HOME/skills` (default `~/.codex/skills`) |
| `command-code` | `.commandcode/skills/` | `~/.commandcode/skills/` |
| `continue` | `.continue/skills/` | `~/.continue/skills/` |
| `cortex` | `.cortex/skills/` | `~/.snowflake/cortex/skills/` |
| `crush` | `.crush/skills/` | `~/.config/crush/skills/` |
| `cursor` | `.agents/skills/` | `~/.cursor/skills/` |
| `deepagents` | `.agents/skills/` | `~/.deepagents/agent/skills/` |
| `devin` | `.devin/skills/` | `$XDG_CONFIG_HOME/devin/skills/` |
| `dexto` | `.agents/skills/` | `~/.agents/skills/` |
| `droid` | `.factory/skills/` | `~/.factory/skills/` |
| `eve` | `agent/skills/` (+ subagents) | N/A |
| `firebender` | `.agents/skills/` | `~/.firebender/skills/` |
| `forgecode` | `.forge/skills/` | `~/.forge/skills/` |
| `gemini-cli` | `.agents/skills/` | `~/.gemini/skills/` |
| `github-copilot` | `.agents/skills/` | `~/.copilot/skills/` |
| `goose` | `.goose/skills/` | `$XDG_CONFIG_HOME/goose/skills/` |
| `grok` | `.grok/skills/` | `$GROK_HOME/skills` (default `~/.grok/skills`) |
| `hermes-agent` | `.hermes/skills/` | `$HERMES_HOME/skills` (default `~/.hermes/skills`) |
| `inference-sh` | `.inferencesh/skills/` | `~/.inferencesh/skills/` |
| `jazz` | `.jazz/skills/` | `~/.jazz/skills/` |
| `junie` | `.junie/skills/` | `~/.junie/skills/` |
| `iflow-cli` | `.iflow/skills/` | `~/.iflow/skills/` |
| `kilo` | `.kilocode/skills/` | `~/.kilocode/skills/` |
| `kimchi` | `.kimchi/skills/` | `~/.config/kimchi/harness/skills/` |
| `kimi-code-cli` | `.agents/skills/` | `~/.agents/skills/` |
| `kiro-cli` | `.kiro/skills/` | `~/.kiro/skills/` |
| `kode` | `.kode/skills/` | `~/.kode/skills/` |
| `lingma` | `.lingma/skills/` | `~/.lingma/skills/` |
| `loaf` | `.agents/skills/` | `~/.agents/skills/` |
| `mcpjam` | `.mcpjam/skills/` | `~/.mcpjam/skills/` |
| `minimax-code` | `.minimax/skills/` | `~/.minimax/skills/` |
| `mistral-vibe` | `.vibe/skills/` | `$VIBE_HOME/skills` (default `~/.vibe/skills`) |
| `moxby` | `.moxby/skills/` | `~/.moxby/skills/` |
| `mux` | `.mux/skills/` | `~/.mux/skills/` |
| `opencode` | `.agents/skills/` | `$XDG_CONFIG_HOME/opencode/skills/` |
| `openhands` | `.openhands/skills/` | `~/.openhands/skills/` |
| `ona` | `.ona/skills/` | `~/.ona/skills/` |
| `pi` | `.pi/skills/` | `~/.pi/agent/skills/` |
| `qoder` | `.qoder/skills/` | `~/.qoder/skills/` |
| `qoder-cn` | `.qoder/skills/` | `~/.qoder-cn/skills/` |
| `qwen-code` | `.qwen/skills/` | `~/.qwen/skills/` |
| `replit` | `.agents/skills/` | `$XDG_CONFIG_HOME/agents/skills/` |
| `reasonix` | `.reasonix/skills/` | `~/.reasonix/skills/` |
| `rovodev` | `.rovodev/skills/` | `~/.rovodev/skills/` |
| `roo` | `.roo/skills/` | `~/.roo/skills/` |
| `tabnine-cli` | `.tabnine/agent/skills/` | `~/.tabnine/agent/skills/` |
| `terramind` | `.terramind/skills/` | `~/.terramind/skills/` |
| `tinycloud` | `.tinycloud/skills/` | `~/.tinycloud/skills/` |
| `trae` | `.trae/skills/` | `~/.trae/skills/` |
| `trae-cn` | `.trae/skills/` | `~/.trae-cn/skills/` |
| `warp` | `.agents/skills/` | `~/.agents/skills/` |
| `windsurf` | `.windsurf/skills/` | `~/.codeium/windsurf/skills/` |
| `zed` | `.agents/skills/` | `~/.agents/skills/` |
| `zcode` | `.zcode/skills/` | `~/.zcode/skills/` |
| `zencoder` | `.zencoder/skills/` | `~/.zencoder/skills/` |
| `zenflow` | `.zencoder/skills/` | `~/.zencoder/skills/` |
| `neovate` | `.neovate/skills/` | `~/.neovate/skills/` |
| `pochi` | `.pochi/skills/` | `~/.pochi/skills/` |
| `promptscript` | `.agents/skills/` | N/A |
| `adal` | `.adal/skills/` | `~/.adal/skills/` |
| `universal` | `.agents/skills/` | `$XDG_CONFIG_HOME/agents/skills/` |

README’s simplified table `./<agent>/skills/` / `~/<agent>/skills/` is **not** literally true for many agents; the table above is the code truth.

### 1.8 Link / copy installation semantics

Modes: `symlink` (default when multiple distinct target dirs and not forced) or `copy` (`--copy`, or automatic when only one unique target dir / all-Eve) ([add.ts L1562–1599](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1562-L1599), [installer.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/installer.ts)).

**Symlink mode:**

1. Clean/recreate canonical dir.
2. Copy skill tree into canonical (excludes `metadata.json`, `.git`, `__pycache__`, `__pypackages__`; dereferences symlinks; skips broken symlinks).
3. For each non-universal agent target, create a relative symlink (Windows: junction) from agent path → canonical.
4. Global universal agents write only to canonical (no extra agent-global symlink).
5. Project non-universal agents whose project root config dir does not already exist are **skipped** for linking (skill still lands in `.agents/skills/`), **except** `claude-code` which is always linked if selected.
6. Symlink failure → fall back to copy into the agent path; warn about Windows Developer Mode.

**Copy mode:**

- Write a full independent copy directly into each agent path; no canonical indirection required for the agent copy (canonical may still exist depending on path).

**Overlap safety:**

- If source skill path overlaps destination (e.g. installing from `./skills` into OpenClaw project `skills/`), install is **skipped** for that target rather than deleting the source ([installer.ts L234–244](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/installer.ts#L234-L244)).

**Overwrite behavior:**

- Existing skill directories/symlinks at the destination are removed and replaced (`cleanAndCreateDirectory` / replace symlink).
- Pre-install summary labels agents that already have the skill as `overwrites:` ([add.ts L1605–1665](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1605-L1665)).
- Confirmation prompt unless `-y`.
- There is **no** merge, three-way conflict UI, or “keep existing” option in `add`. Overwrite is all-or-nothing after confirm.

**`skills use`:** resolves sources like `add`, materializes selected skill files to a temp dir, prints a generated prompt to stdout, or starts one agent with `--agent` ([use.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/use.ts); README).

### 1.9 Authentication behavior

Upstream does **not** store tokens in its own config.

| Mechanism | Use |
|---|---|
| Ambient git credentials | HTTPS/SSH clones via system git |
| `gh` CLI | `gh auth status` / `gh repo clone` / `gh auth token` |
| `GITHUB_TOKEN` / `GH_TOKEN` env | Preferred silent GitHub API auth |
| SSH keys | SSH URLs; HTTPS auth-failure fallback with `BatchMode=yes` |
| `GH_HOST` | GitHub Enterprise hostname for shorthand |
| SAML SSO errors | Explicit remediation message (re-auth SSO or use SSH) |

GitHub API token acquisition is **lazy**: unauthenticated first; only on rate-limit (403 + remaining 0) or private-repo 401/404 does it call `getGitHubToken()`, which may run `gh auth token` once per process with a stderr notice ([skill-lock.ts L140–179](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skill-lock.ts#L140-L179), [blob.ts L166–176](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/blob.ts#L166-L176)).

Well-known and direct downloads use anonymous HTTP `fetch` (no auth hooks).

Telemetry is skipped for private GitHub repos (and when privacy cannot be determined) ([add.ts L1838–1865](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1838-L1865)).

### 1.10 Version / lock / update metadata and commands

Two lockfiles:

#### Global lock — `~/.agents/.skill-lock.json` (or `$XDG_STATE_HOME/skills/.skill-lock.json`)

- Written **only for successful global installs** with a normalized remote source ([add.ts L1868+](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1868)).
- Schema version **3** (older versions wiped) ([skill-lock.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skill-lock.ts)).
- Per skill:
  - `source` (owner/repo or preserved SSH/non-github URL)
  - `sourceType`
  - `sourceUrl`
  - `ref?`
  - `skillPath?` (path to SKILL.md in source)
  - `skillFolderHash` (GitHub tree SHA of skill folder when available; else content hash)
  - `installedAt` / `updatedAt`
  - `pluginName?`
  - `sourceBaseUrl?` / `wellKnownDigest?` for well-known
- Also stores `dismissed` prompts and `lastSelectedAgents`.

#### Project lock — `<cwd>/skills-lock.json`

- Written for successful **project** installs that are not pure direct-download ([add.ts L1890–1928](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts#L1890-L1928)).
- Schema version **1**; intended to be committed; alphabetically sorted keys; no timestamps ([local-lock.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/local-lock.ts)).
- Per skill:
  - `source`, optional `sourceUrl` (required effectively for `git`/`gitlab` restore)
  - `ref?`
  - `sourceType`
  - `skillPath?`
  - `computedHash` (SHA-256 over all files in skill folder, paths included)
  - `subagents?` (Eve placement)
  - `wellKnownDigest?`
- Local sources are written as portable relative paths when possible.

**`skills update`** ([update.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/update.ts)):

- Options: `-g` global only, `-p` project only, `-y` auto scope, optional skill name filters.
- **Global GitHub:** Trees API folder hash vs `skillFolderHash`; on change re-runs `skills add <source> --skill <name> -g -y`.
- **Global non-GitHub remote:** clone + recompute folder hash.
- **Well-known:** compare digests; reinstall when changed.
- **Skipped global entries:** `local`, bare `git` without checkable hash path, incomplete lock fields — printed with manual reinstall hint.
- **Project:** uses `skills-lock.json`; skips `node_modules` and `local` sourceTypes for auto-update; rebuilds install URL via `buildLocalUpdateSource`; may pass `--full-depth` when skill path cannot be safely appended to the source URL.
- Upstream deletions can be prompted for removal.
- Update child processes never use a shell (argv-only spawn).

**`skills experimental_install`:** restores from `skills-lock.json` into universal `.agents/skills/` only; `node_modules` entries go through `experimental_sync` ([install.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/install.ts)).

**`skills experimental_sync`:** discovers skills inside `node_modules` (package root `SKILL.md`, `skills/*`, `.agents/skills/*`) and installs into agent dirs; records `sourceType: 'node_modules'` in the project lock ([sync.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/sync.ts)).

### 1.11 Safety / conflict-related behavior

| Behavior | What upstream does |
|---|---|
| Path traversal in subpaths / skill names / archive members | Rejected or sanitized |
| Install overwrite | Replaces managed skill destination after confirm; no content merge |
| Source/destination path overlap | Skip install for that target |
| Internal skills | Hidden unless env or explicit name request |
| Private repo telemetry | Suppressed when private or unknown |
| Archive bombs | Size/file-count limits |
| Git protocol | Allowlist; no `ext::` |
| Well-known v2 digests | Must match `sha256:…` |
| Security advisory UI | Optional Socket/audit display from telemetry service; advisory only, does not block |
| Post-install warning | “Review skills before use; they run with full agent permissions.” |
| Find-skills upsell | One-time global prompt after interactive install |

`remove` deletes installed skill dirs/links for selected agents/scopes and updates lockfiles ([remove.ts](https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/remove.ts)). It does not implement a general “unmanaged file conflict” model beyond operating on skill install paths.

### 1.12 Skill authoring surface (as consumed)

From README + `parseSkillMd`:

- Required frontmatter: `name`, `description`.
- Optional: `metadata.internal`.
- README mentions Agent Skills specification compatibility and agent-specific optional features (`allowed-tools`, `context: fork`, hooks) in a compatibility table; those features are **agent runtime** concerns, not enforced by this CLI beyond passing files through (Eve frontmatter stripping is the main CLI-side transform).

---

## 2. Implications for Skill Manager (not decisions)

These are consequences of the facts above for a product that wants **compatibility** with the vercel-labs/skills ecosystem. They are not Skill Manager product choices.

1. **Source locator parity** means accepting at least: local paths; GitHub shorthand/URLs/tree/subpath/`@skill`/`#ref`; GitLab URLs; generic git/SSH; well-known HTTP endpoints; direct SKILL.md/archive URLs. Partial support is a deliberate subset, not full CLI parity.
2. **Discovery parity** is more than “find `**/SKILL.md`” — depth-3 containers, priority dirs, plugin manifests, internal-skill gating, installed-lock skipping, and `--full-depth` fallback are part of the observed behavior.
3. **Install path parity** cannot use a single `~/<agent>/skills` rule. Universal agents share `.agents/skills`; many agents have asymmetric project vs global paths and env overrides.
4. **Canonical + symlink** is the CLI’s multi-agent distribution model. Skill Manager’s domain model (Skill Store + Assignments + symlink Distribution) is conceptually close to that pattern, but Skill Manager’s Store is product-owned authority, whereas the CLI’s canonical `.agents/skills` is itself a consumer-facing install location.
5. **Lockfiles are dual and scope-split.** Global tracking lives under `~/.agents/.skill-lock.json` with GitHub tree SHAs; project tracking is commit-friendly `skills-lock.json` with content hashes. Interop with either is optional and format-version-sensitive (global v3 wipe-on-upgrade).
6. **Auth posture matches Skill Manager’s stated constraint** (reuse git/`gh`/env credentials; do not store tokens) if Skill Manager mirrors the lazy ambient-credential approach.
7. **Overwrite semantics differ from Skill Manager’s stated Distribution rule** (“never overwrite unmanaged content”). The CLI overwrites existing skill destinations after confirm. Compatibility with CLI-installed trees may require detecting foreign/unmanaged paths carefully.
8. **Blob/skills.sh fast path and well-known v0.2 digests** are ecosystem-specific accelerators/trust mechanisms; they are not required to consume git-hosted skills, but matter for parity with popular public sources and publisher hosting.
9. **Agent name strings** (`claude-code`, `pi`, …) are a de facto public vocabulary for targets; aligning Target identifiers with these keys reduces user confusion when both tools are used.
10. **`experimental_sync` / node_modules skills** are a third source class beyond git/local/HTTP; MVP source scope may exclude them without breaking core git/local compatibility.

---

## 3. Gaps and behavior not guaranteed by upstream

- **No stable public library API.** Compatibility is defined by CLI behavior and on-disk layouts, not a versioned SDK contract. Anything not covered by README + source at a given commit may change without notice.
- **Agent list and paths churn.** The 76-agent table is commit-specific; new agents and path tweaks land frequently.
- **Blob allowlists and skills.sh hosting** are operational configuration inside the CLI, not a general promise that every GitHub repo can install via blob.
- **Root-level skill supporting files** differ between blob path (SKILL.md only) and clone path (full directory copy). Behavior is not uniform.
- **Discovery dir list ≠ install path list.** Skills living only under e.g. `.github/skills` may be discovered as sources but that path is not an install target for a first-class agent key.
- **Well-known schema evolution.** Only legacy (no schema) and exactly `discovery/0.2.0` are accepted; future schema versions are ignored until the CLI learns them.
- **GitLab / generic git update restore** requires `sourceUrl` in project lock; older shorthand-only entries can be unrestorable.
- **No formal conflict protocol** for divergent local edits vs upstream beyond hash mismatch → reinstall overwrite, and optional deletion prompts on update.
- **Private well-known / authenticated HTTP downloads** are not a first-class authenticated surface.
- **Windows** is partially handled (junctions, path sanitization) but Skill Manager MVP explicitly outscopes Windows; CLI Windows behavior was not exhaustively verified here.
- **Telemetry / security audit endpoints** (`skills.sh`, Socket-related audit fetch) are best-effort and non-blocking; their response schemas are not treated as a compatibility contract for Skill Manager.
- **Exact interactive UX** (prompts, multiselect, find-skills upsell) is CLI UX, not an on-disk compatibility requirement.
- **Agent runtime loading rules** (when an agent actually picks up a skill, hooks, `allowed-tools`, etc.) are owned by each agent’s docs, not fully specified by this CLI beyond install locations.

---

## 4. Source index (durable links at inspected commit)

| Topic | Permalink |
|---|---|
| README | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/README.md |
| package.json | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/package.json |
| Source parser | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/source-parser.ts |
| Skill discovery | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skills.ts |
| Plugin manifests | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/plugin-manifest.ts |
| Agents + paths | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/agents.ts |
| Types | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/types.ts |
| Installer | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/installer.ts |
| Add flow | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/add.ts |
| Git clone / auth | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/git.ts |
| Direct download | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/download-source.ts |
| Well-known provider | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/providers/wellknown.ts |
| Blob / skills.sh | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/blob.ts |
| Global lock | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/skill-lock.ts |
| Project lock | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/local-lock.ts |
| Update | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/update.ts |
| Update source rebuild | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/update-source.ts |
| Lock restore | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/install.ts |
| node_modules sync | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/sync.ts |
| CLI surface | https://github.com/vercel-labs/skills/blob/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5/src/cli.ts |
| Commit | https://github.com/vercel-labs/skills/commit/a4d243c3d4f86cdf9385dd1b6a0733f6937e70b5 |
