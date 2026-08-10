# Define the Target adapter contract

Type: grilling
Status: resolved
Blocked by: 01

## Question

Which Agent adapters and custom Targets must the MVP ship, and how should each adapter resolve user-level and project-level Target paths while preserving safe custom-path behavior?

## Context

- [Research Vercel Skills compatibility surface](01-research-vercel-skills-compatibility.md)

## Answer

The MVP ships a curated, versioned set of eight Target Adapters rather than the upstream 76-Agent matrix. Unsupported Agents use a custom Target.

### Built-in Target Adapters

| Target Adapter | Project Target | User Target |
|---|---|---|
| Universal | `<project>/.agents/skills` | `~/.agents/skills` |
| Claude Code | `<project>/.claude/skills` | `${CLAUDE_CONFIG_DIR:-~/.claude}/skills` |
| Codex | `<project>/.agents/skills` | `${CODEX_HOME:-~/.codex}/skills` |
| Cursor | `<project>/.agents/skills` | `~/.cursor/skills` |
| Gemini CLI | `<project>/.agents/skills` | `~/.gemini/skills` |
| OpenCode | `<project>/.agents/skills` | `${XDG_CONFIG_HOME:-~/.config}/opencode/skills` |
| Pi | `<project>/.pi/skills` | `~/.pi/agent/skills` |
| GitHub Copilot | `<project>/.agents/skills` | `~/.copilot/skills` |

Universal deliberately uses the ecosystem-wide `~/.agents/skills` user path. All built-in Target Adapters support both project-level and user-level Target creation.

### Resolution and identity

A Target Adapter is a path-discovery aid, not a Target's identity or owner. Target creation resolves and stores a normalized absolute path plus explanatory creation metadata such as the requested adapter, scope, and project root. Existing Targets never move merely because the current working directory, environment, or a later adapter version changes.

- A project Target requires an explicit, existing directory as its project root. The CLI may propose the current directory. The root's existing symbolic links are resolved before the Target path is fixed; an Agent-specific Skills directory need not exist yet.
- A user Target resolves `HOME` and any adapter-specific override when it is created. Empty overrides count as unset. A non-empty relative `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, or `XDG_CONFIG_HOME` is an error rather than a reason to silently use the default.
- The normalized absolute path is unique across Targets. Creating another Target that resolves to the same path returns the existing Target.
- A shared Target does not persist Agent ownership. The WebUI and CLI derive compatible adapters from the current adapter table. For example, one project `.agents/skills` Target may be shown as compatible with Universal, Codex, Cursor, Gemini CLI, OpenCode, and GitHub Copilot.

### Custom Targets and path safety

A custom Target directly names the Skills container: Skills distribute to `<custom-path>/<slug>`. It is not a project root and has no hidden adapter-specific suffix.

- Input may be an absolute path or `~/...`; `~` is expanded once and the normalized absolute result is stored.
- Environment variables, glob patterns, and another user's `~name` are not expanded.
- The container may be absent. Registering a Target performs no filesystem mutation; existence remains observable Target state until Distribution acts.
- An existing non-directory path is invalid. Existing path prefixes are resolved through symbolic links before validation.
- The filesystem root is invalid. Every built-in or custom Target and the Skill Store must be disjoint after resolution: they cannot be equal or stand in an ancestor/descendant relationship.
- Safety boundaries are re-evaluated before every Distribution so a symbolic link introduced after registration cannot redirect writes into the Skill Store.
- Skill Manager never deletes a Target's container directory.

### Agent installation detection

Installation detection ships in the MVP but is advisory. It prioritizes and may preselect adapters during `skillctl init`, WebUI setup, and Target creation, but it neither creates a Target automatically nor prevents a user from selecting an undetected adapter. The user confirms every Target before it is registered.

Detection is read-only and computed on demand rather than stored as durable state. Each result includes `detected`, `not_detected`, `unknown`, or `not_applicable`, the evidence that produced it, and the detection time. Inspection failures produce `unknown`.

An adapter is `detected` when any of these signals exists:

1. Its executable is found on `PATH`.
2. A known macOS application entry exists.
3. Its user configuration root contains any entry other than the `skills` directory. A Skills directory alone is not evidence because Skill Manager may have created it.

| Target Adapter | Executable | macOS application evidence | User configuration root |
|---|---|---|---|
| Claude Code | `claude` | — | `${CLAUDE_CONFIG_DIR:-~/.claude}` |
| Codex | `codex` | `ChatGPT.app`; legacy `Codex.app` | `${CODEX_HOME:-~/.codex}` |
| Cursor | `cursor` | `Cursor.app` | `~/.cursor` |
| Gemini CLI | `gemini` | — | `~/.gemini` |
| OpenCode | `opencode` | — | `${XDG_CONFIG_HOME:-~/.config}/opencode` |
| Pi | `pi` | — | `~/.pi/agent` |
| GitHub Copilot | `copilot` | — | `~/.copilot` |
| Universal | — | — | — |

macOS application evidence is checked in the standard system and per-user Applications directories. Project files are not Agent-installation evidence. Universal and custom Targets return `not_applicable` because neither represents one concrete detectable Agent.

The glossary now records **Target Adapter** in [`docs/CONTEXT.md`](../../../docs/CONTEXT.md).
