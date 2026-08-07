# Define the Source and import contract

Type: grilling
Status: resolved
Blocked by: 01

## Question

What canonical Source locators, repository or directory layouts, Skill discovery and selection rules, slug-conflict behavior, and Source-to-Skill binding operations should the MVP expose?

## Context

- [Research Vercel Skills compatibility surface](01-research-vercel-skills-compatibility.md)

## Answer

### Supported Source kinds

MVP exposes two underlying Source kinds:

- **Local Source** — a local directory.
- **Git Source** — GitHub, GitLab, or another SSH/HTTPS Git repository. GitHub receives `owner/repo` shorthand and ambient `gh` authentication conveniences, but remains a Git Source.

Well-known indexes, direct `SKILL.md` or archive downloads, and `node_modules` are out of the MVP.

User input is normalized into structured fields: `kind`, `location`, optional `ref`, and optional `subpath`. Local locations become canonical absolute real paths; Git locations become full Git URLs. Skill selection is separate from the Source locator, so the MVP does not store Vercel-style `@skill` or `#ref@skill` compound syntax.

A Source has an immutable internal ID and editable display name. The normalized tuple `kind + location + ref + subpath` is unique. The same repository may be registered more than once only when its `ref` or `subpath` differs.

### Git refs and authentication

- Omitted `ref` follows the remote default branch.
- A named branch or tag is resolved again on each check.
- A full commit SHA is pinned and does not report upstream updates.
- Local Sources cannot have a `ref`.
- The latest resolved full commit SHA is recorded.
- Authentication uses ambient Git/SSH/HTTPS credentials, `gh`, or environment tokens; Skill Manager never stores credentials.

### Discovery and inventory

Discovery starts after applying `subpath`:

1. If that root directly contains `SKILL.md`, it is one Skill and discovery stops.
2. Otherwise scan for `SKILL.md` up to three directory levels below the root. This covers `skills/<skill>`, `.agents/skills/<skill>`, and one classification level.
3. Once a Skill is found, do not descend into its directory.
4. Skip `.git`, `node_modules`, build output, and cache directories.
5. Do not parse Claude plugin manifests or offer unlimited recursion. A deeper catalog must register a narrower `subpath`.

A valid Skill requires non-empty YAML frontmatter `name` and `description`. Invalid entries are reported and skipped without hiding other valid entries.

Each inventory entry is identified by its relative Skill directory, not only by frontmatter name. CLI `--skill <name>` is accepted only when the name is unique within that Source; ambiguous names require `--path <relative-directory>`. WebUI shows both name and relative path.

Adding a Source validates access, scans it, and stores its inventory, but does not modify the Skill Store. Import is always explicit through selected entries, `--import <...>`, or `--import-all`; `--yes` never implies import-all. A Source may exist without imported Skills.

An existing Source that later becomes inaccessible is retained with `unavailable`, the error, the last successful check time, and a visibly stale last inventory. It cannot import or synchronize until reachable; existing Store content and Distribution remain available. A Source that cannot be reached and scanned during initial registration is not saved.

### Full Skill materialization

Import copies the complete Skill directory, including `references`, `scripts`, `assets`, binaries, and any other files. It preserves relative structure and executable permissions; Source `.git` metadata is excluded.

Directory-internal symlinks are dereferenced so the Store copy is self-contained. A broken link or a link resolving outside the Skill root rejects the entire Skill rather than producing a partial copy. Path traversal, device files, sockets, and other special files are always rejected.

Git Sources do not automatically initialize submodules or fetch Git LFS objects. A selected Skill containing an unmaterialized gitlink or LFS pointer fails import with remediation guidance. A Local Source whose content is already materialized as ordinary files imports normally.

Default per-Skill guards are 10,000 files, 100 MiB total, and 50 MiB per file. Exceeding a size/count guard requires explicit `--allow-large` or WebUI confirmation; unsafe paths and special files cannot be overridden.

### Slug and Store content

A Skill slug defaults from frontmatter `name`, normalized to lowercase ASCII letters, numbers, and single hyphens, with a 64-character maximum. Users may override it before import. Slugs are globally unique in the Skill Store, and import never rewrites upstream `SKILL.md`.

Skill directories contain only Skill-owned content. Source Binding, resolved version, hashes, and timestamps live in SQLite rather than injected metadata files. If SQLite is lost, Store directories can be recovered as unbound Skills, while Source, Group, and Assignment state requires database backup.

A Local Source and the Skill Store must not overlap in either direction after resolving real paths.

### Slug conflicts and replacement

- The same Source entry already bound to the same Skill is an already-imported no-op.
- A different entry claiming an existing slug is skipped by default.
- The user may choose a different slug or explicitly **Replace** the existing Skill.
- Replace previews affected Groups and Targets, requires confirmation, snapshots old content, then changes content and Source Binding while retaining the slug, Group memberships, and Assignments.
- Batch import is per-Skill: conflicting or invalid entries fail independently and do not roll back successful imports.

### Source Binding lifecycle

A Source Binding stores `Source ID + upstream relative Skill directory`. Each Skill has at most one Binding.

- Import establishes the Binding.
- **Detach** removes it but retains the Skill, Groups, Assignments, and distributed links; Sync Status becomes `unbound`.
- **Rebind** explicitly chooses an inventory entry from another Source. Content differences enter the synchronization conflict flow and never overwrite silently.
- Deleting a bound Source is blocked by default. Explicit deletion may first detach all bound Skills, but never deletes those Skills.

### Transaction boundary

Each Skill import stages the complete tree, validates it, computes its digest, and atomically moves it into the Skill Store. A failure or crash leaves no partial Skill. Replace snapshots old content before atomic replacement. SQLite commits the Binding and digest only after the filesystem replacement succeeds. Batch operations retain the per-Skill transaction boundary.
