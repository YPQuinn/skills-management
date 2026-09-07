# Safety

Skill Manager never overwrites unmanaged files and never silently adopts someone else's Target entries. Deletion, detach, and rebind print a preview and require `--yes` or a TTY confirmation. Synchronization, conflict actions, rollback, and Distribution are explicit commands but are not confirmation-gated.

## Source versus Store

A **Source** is upstream only: a local directory or a Git repository Skill Manager can scan. Registering a Source records its **Source Inventory**. It does not copy anything into the Store.

The **Skill Store** is the only authoritative copy. Imports copy a full Skill tree into `~/.skillctl/store/<slug>` (or the custom `--store` path). Agent Targets never link at a Source; they link at the Store.

A **Source Binding** associates one managed Skill with one Inventory entry (`Source` + relative directory). Removing the Binding leaves the Skill in the Store. Deleting a Source with bound Skills is blocked unless `--detach-skills` is passed; those Skills are detached, not deleted.

## Synchronization versus Distribution

**Synchronization** is one-way retrieval from a Source into the Skill Store. It is never scheduled and never pushed back upstream.

```text
skillctl skill check <slug>
skillctl skill diff <slug>
skillctl skill sync <slug>
skillctl source sync <source>
```

`check` refreshes observations only. `sync` auto-updates only `source_changed`. `store_changed` and `conflict` are skipped until an explicit action. Batch `source sync` uses one fresh Source check and skips the same states.

**Distribution** reconciles a Target with its **Assignments**. Assignment changes desired state only; no Target file is created or removed until `skillctl target distribute`.

```text
skillctl target distribute <target> --dry-run
skillctl target distribute <target>
```

Distribution creates missing desired links, then removes **Managed Links** that are no longer desired. It never writes Source content onto a Target.

## Unmanaged content

A **Managed Link** is a Target symlink Skill Manager created or that was **Adopted**, and whose recorded raw target and physical identity (device, inode, and symlink modification time) still match. A replacement symlink is not owned merely because it points to the same Store path. Only a Managed Link may be changed or removed.

Distribution:

- never overwrites an existing file, directory, or foreign symlink
- never enumerates or deletes unrelated Target names
- never deletes the Target container
- leaves an entry whose ownership can no longer be proven (`ownership_lost`)

An unmanaged same-name file is a Target **conflict** and remains byte-for-byte unchanged.

## Conflicts

Two different conflict kinds exist.

**Synchronization conflict.** Source and Store both differ from the **Synchronization Baseline**. Ordinary `sync` skips the Skill. Inspect with `skillctl skill diff <slug>`, then choose:

| Action | Command | Effect |
| --- | --- | --- |
| Keep Store | `skillctl skill keep-store <slug>` | Store bytes unchanged; current Source becomes the new Baseline |
| Accept Source | `skillctl skill accept-source <slug>` | Snapshot the Store, then replace it with validated Source content |
| Detach | `skillctl skill detach <slug>` | Drop the Binding; the Store copy stays as an unbound Skill |

There is no per-file merge and no implied Accept Source from `--yes`.

**Target conflict.** The slug path exists but is not the expected Managed Link. Distribution reports `blocked_conflict` and leaves the entry alone. Repair the path by hand, or **Adopt** if it is an eligible symlink.

## Adopt

An unmanaged symlink is never adopted implicitly, even when it already points at the correct Store Skill.

```text
skillctl target adopt <target> <slug>
```

Adoption succeeds only when a fresh physical resolution of that symlink is exactly the currently desired Store Skill. The link is not rewritten; its existing raw target and physical identity are recorded as ownership. Files, directories, wrong-target links, and links that do not resolve into the Store cannot be adopted.

## Snapshots and rollback

Store replacements (import replace, `sync` of `source_changed`, and `accept-source`) keep **one** previous snapshot of live content. That snapshot is recovery and one-step rollback state, not version history.

```text
skillctl skill rollback <slug>
```

Rollback restores the previous snapshot. The displaced live tree becomes the new previous snapshot, so a second rollback undoes the first. Binding, Group, and Assignment relationships do not change. Existing Target Managed Links keep pointing at the same Store path; their bytes follow the restored Store content.

If there is no snapshot, rollback is blocked.

## Deletion

Referenced Skill deletion is blocked by default. `--cleanup` first removes related Assignments and verifiable Managed Links. Paths whose ownership can no longer be proven are left untouched.

Target deletion removes verifiable Managed Links, then drops the registration. The container stays. Group deletion is blocked while assigned unless `--unassign`; leftover Managed Links wait for the next explicit Distribution.
