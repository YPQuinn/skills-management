# Prototype the Skill Manager operator workflow

Type: prototype
Status: resolved
Blocked by: 03, 04, 05, 06

## Question

What CLI command language and Appica WebUI information architecture let a user import and organize Skills, configure Targets, and understand and act on synchronization and Distribution Status without ambiguity?

## Prototype

- [Appica operator-workflow prototype](../assets/operator-workflow-prototype.md) — three in-memory variants comparing an operations inbox with task-first commands, a resource explorer with noun-first commands, and a delivery plan with plan/apply commands; preserved on `prototype/operator-workflow` at `9cbf486`.

## Comments

- Selected **Resource explorer** as the default WebUI entry and primary information architecture.

## Answer

### Prototype verdict

The **Resource explorer** variant becomes the WebUI foundation and the noun-first command language becomes the CLI foundation. The Operations inbox is not a separate default page, and plan/apply is not the product-wide interaction model. The useful part of planning is retained as a local dry-run and change summary for Distribution.

The validated Appica prototype is preserved as a primary-source asset on the throwaway `prototype/operator-workflow` branch at commit `9cbf486`; prototype code does not remain on `main`.

### WebUI information architecture

After setup, Skill Manager opens directly on **Skills**. The persistent top-level navigation contains exactly:

- Skills
- Sources
- Groups
- Targets

MVP has no generic Home, Dashboard, Operations inbox, Activity, or standalone Assignments area. State needing attention is expressed through navigation counts, list columns, filters, and resource details rather than duplicated into another page.

The desktop shell uses the Resource explorer's responsive master–detail model: top-level navigation at the left, the current resource list in the middle, and the selected resource detail at the right. Selection, detail tabs, filters, sorting, and search all update stable URLs so views can be refreshed, linked, and navigated with browser history. Representative routes are:

```text
/skills
/skills/code-review
/skills/code-review?tab=synchronization
/skills?filter=needs-attention
/sources/vercel
/groups/engineering
/targets/pi-user
```

On narrow screens, list and detail become separate route views rather than compressed columns. Deleting or renaming returns to the relevant list while retaining still-valid filters. Relationships navigate to their owning resource rather than duplicating full detail.

Skill detail has **Overview**, **Synchronization**, and **Distribution** tabs. Source detail owns availability, checks, Source Inventory, invalid entries, Source Bindings, and import selection. Group detail owns membership and links to assigned Targets. Target detail owns adapter/path identity, direct Skill and Group Assignments, the expanded desired Skill set, live Distribution Status, dry-run, Distribution, and eligible adoption.

### Status presentation

Synchronization and Distribution are never collapsed into a single health score. The Skills list keeps separate **Synchronization** and **Targets** columns. Badges always contain text and use these operator-facing labels:

- `In sync`
- `Source changed`
- `Store changed`
- `Sync conflict`
- `Linked`
- `Missing`
- `Target conflict`
- `Broken link`

Source availability, observation staleness, and last-inspected time remain separate from the last observed relationship. Detail views expose the technical status, latest operation result, timestamps, and actionable error. `Needs attention` covers conflicts, invalid or missing content, Source or Store changes, broken or missing links, and stale observations while preserving their categories.

Each state displays only legal actions. A Sync conflict offers **View diff**, **Keep Store**, and **Accept Source**. A Target conflict offers **Inspect** and **Adopt** only when the symlink is eligible. Appica Badge, Alert, Table, Dialog, and Alert Dialog patterns carry text semantics; color is never the sole signal.

### Source and import workflow

Source registration and Skill import are separate operations in both interfaces. Adding a Source validates and checks it, then opens Source detail with its Source Inventory, invalid-entry reports, relative paths, and selection controls. **Import selected** reports per-entry results and links to resulting Skill details.

The CLI equivalent is:

```text
skillctl source add github:vercel-labs/agent-skills --name vercel
skillctl source show vercel
skillctl source check vercel
skillctl skill import --source vercel --path skills/release-notes
skillctl skill import --source vercel --path skills/release-notes --slug release-notes
skillctl skill import --source vercel --all
```

`source add` never imports implicitly and receives no MVP import convenience flags. `skill import --all` is explicit; `--yes` cannot select all. A stable relative `--path` identifies an entry, while `--skill <name>` is accepted only when unique within that Source. A single-entry import may choose `--slug` or explicit `--replace`; Replace is forbidden with `--all`.

### Assignments and Groups

Assignment remains the domain object but not a fifth operator-facing resource. It is managed from Target detail and the `target` command group:

```text
skillctl target assign <target> --skill <slug>
skillctl target assign <target> --group <name>
skillctl target unassign <target> --skill <slug>
skillctl target unassign <target> --group <name>
```

Target detail distinguishes direct Skill Assignments, Group Assignments, their expanded union, and every reason a Skill is desired. Group membership belongs to Group detail and:

```text
skillctl group add-skill <group> <slug...>
skillctl group remove-skill <group> <slug...>
```

### Synchronization workflow

Safe synchronization remains the default. Conflict choices stay within the `skill sync` vocabulary rather than introducing a generic `resolve` command:

```text
skillctl skill diff code-review
skillctl skill diff code-review --path SKILL.md
skillctl skill sync code-review
skillctl skill sync code-review --keep-store
skillctl skill sync code-review --accept-source
skillctl skill sync --all
skillctl skill rollback code-review
```

Plain `skill sync` only applies safe `source_changed` content. `--all` still skips `store_changed` and `conflict`. `--keep-store` and `--accept-source` are mutually exclusive, require one explicit Skill, and are never inferred from `--yes`. WebUI uses the same action names and shows their CLI equivalents.

### Distribution workflow

Distribution uses Target-scoped status, optional dry-run, and direct safe reconciliation rather than a global plan/apply protocol:

```text
skillctl target status pi-user --refresh
skillctl target distribute pi-user --dry-run
skillctl target distribute pi-user
skillctl target distribute --all
skillctl target adopt pi-user wayfinder
```

`target distribute` prints a concise plan summary and immediately executes all safe items. `--dry-run` prints full per-item intent without mutation. Conflicts and broken links remain skipped. Removal of no-longer-desired Managed Links is normal reconciliation and appears explicitly in the summary. Adopt is always a separate explicit command and can never be implied by `--yes`.

Target detail displays current state and the change summary before its single **Distribute** action; it does not add a redundant confirmation dialog. Consequential deletion, Accept Source, Replace, and similar destructive choices use Appica Alert Dialog.

### First-run workflow

`skillctl init` confirms the configured Store, config, and SQLite paths, creates local state, shows detected Target Adapters, and lets the user explicitly select suggested Targets. It does not add Sources, import Skills, create Groups, or run Distribution.

When state is absent, `skillctl ui` opens `/setup` with three short steps: **Storage**, optional detected **Targets**, and **Ready**. Completion enters `/skills`; Source onboarding begins from **Add Source**. `skillctl ui` runs a foreground localhost service, opens the browser by default, accepts `--no-open` and `--port`, and stops with its process. It never installs a daemon.

### CLI command language

CLI commands use singular noun groups and mirror WebUI ownership:

```text
skillctl init
skillctl status
skillctl ui

skillctl source add
skillctl source list
skillctl source show
skillctl source check
skillctl source delete

skillctl skill list
skillctl skill show
skillctl skill import
skillctl skill diff
skillctl skill sync
skillctl skill rollback
skillctl skill detach
skillctl skill rebind
skillctl skill delete

skillctl group create
skillctl group list
skillctl group show
skillctl group add-skill
skillctl group remove-skill
skillctl group delete

skillctl target add
skillctl target list
skillctl target show
skillctl target status
skillctl target assign
skillctl target unassign
skillctl target distribute
skillctl target adopt
skillctl target delete
```

`list` emits a compact human table and `show` emits full detail. Every read and mutation result supports `--json`. Batch scope always requires explicit `--all`. Consequential operations may use `--yes` to skip confirmation, but it never selects scope, Replace, Adopt, Keep Store, or Accept Source. Source deletion uses explicit `--detach-skills` when bindings exist. Global `status` is read-only and summarizes Sources, Sync Status, Target state, and recent failures.

MVP does not add top-level `check`, `sync`, `distribute`, `assignment`, `config`, `plan`, or `apply` commands, and it has no Force-overwrite, daemon, or watch vocabulary.
