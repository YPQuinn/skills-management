# Troubleshooting

Successful human output is written to stdout. Diagnostics and human error messages go to stderr. `--json` failures print exactly one JSON error value on stdout. Exit `1` is a blocked or failed use case; exit `2` is argument or configuration validation.

## Credentials

Git Sources invoke `git` with the ambient environment (`GIT_TERMINAL_PROMPT=0`). Skill Manager never stores, prompts for, or writes tokens.

If `source add` or `source check` fails to authenticate:

1. Confirm the same URL works with `git ls-remote <url>`.
2. For SSH remotes, confirm the agent has a loaded key (`ssh-add -l`).
3. For HTTPS GitHub, confirm a credential helper or `gh auth setup-git` already works for that host.
4. `gh` is optional. Installing it does nothing unless Git is already configured to use it.

Private-repository access is whatever Git already has. Skill Manager will not hang waiting for a password prompt.

Local Sources do not call `git`. A Git working tree registered with `--kind local` is observed as current files, not as a repository.

## Source unavailability

An unreachable Source does not erase the last successful Inventory, Bindings, Groups, or Assignments. The last content relationship is kept and marked stale.

```text
skillctl source check <name>
skillctl source show <name>
skillctl skill check <slug>
```

`source show` prints `Status: unavailable` and the last successful check time. `skill sync` and `source sync` block retrieval while the Source is down; they do not rewrite Store content.

If an Inventory entry disappears, the bound Skill becomes `source_missing`. Store content and Managed Links stay. Skill Manager never infers a rename. Wait for the path to return, `skillctl skill rebind`, or `skillctl skill detach`.

## Target conflicts

`target status --refresh` shows `conflict` when the slug path exists but is not the expected Managed Link. `target distribute` reports `blocked_conflict` and does not touch that entry.

Typical causes:

- a same-name file or directory already in the Agent skills folder
- a symlink that points somewhere other than the desired Store Skill
- a Managed Link the user replaced after Distribution

Repair options:

- move or delete the unmanaged entry, then distribute again
- `skillctl target adopt <target> <slug>` when the existing symlink already resolves to the desired Store Skill
- unassign the Skill if it should not be present

The conflicting bytes are left unchanged in every case.

## Broken links

`broken_link` means Skill Manager still owns the symlink, but the destination is missing, invalid, or unsafe. Distribution will not replace a broken link with a new one and will not create a desired Skill whose Store tree is missing or invalid.

```text
skillctl skill show <slug>
skillctl target status <target> --refresh
```

Restore Store content (re-import, `accept-source`, or `rollback` when a snapshot exists), then distribute. Removing a no-longer-desired broken Managed Link is allowed: `unassign` then `distribute`.

## Locks

Concurrent writers fail immediately instead of waiting:

| Message | Meaning |
| --- | --- |
| `another skillctl process is already modifying the installation` | `init` or `--recover-store` lost `~/.skillctl/skillctl.lock` |
| `another skillctl process is writing the Skill Store` | a Store mutation holds `~/.skillctl/locks/store.lock` |

JSON code is `locked`. Retry after the other `skillctl` or WebUI command finishes. Stale lock files from a crashed process are released when that process exits; do not delete lock files while a command is running.

`--json` plus a confirmation-gated command without `--yes` fails with `confirmation required; pass --yes`.

## Recovery

### `state_missing`

Configuration exists, `state.db` does not. Ordinary commands refuse to create empty state.

```text
skillctl status
skillctl init --recover-store
```

`--recover-store` scans the configured Store, adopts valid top-level Skill directories as newly identified unbound Skills (directory name = slug), and reports Sources, Groups, Assignments, Targets, and Managed Link ownership as unrecoverable. Existing Target links become unmanaged. Internal Baseline/snapshot trees are preserved and reported, not guessed or deleted. Store file bytes are not rewritten.

`--recover-store` cannot be combined with `--store`. It is refused unless status is exactly `state_missing`.

The WebUI setup screen offers the same recovery when it detects `state_missing`.

### `recovery_failed`

An unfinished Store operation cannot be proven valid. Skill Manager preserves every candidate tree and blocks new Store writes. Do not delete staging or snapshot directories by hand. Retry after the competing process exits; if the error persists, the leftover evidence under the Store is the recovery input, not trash.

An interrupted Target link creation can also report `recovery_failed`, naming its intent and `.skillctl-r-…` staging directory. If the staged link's identity was not durably recorded before the interruption, or staging has changed, recovery keeps both the directory and its database intent and refuses to proceed. It never infers ownership from a matching destination. Missing staging or staging matching the recorded identity can be cleaned automatically; a published link is accepted only if it matches the recorded identity.

For a persistent staging error, stop all Skill Manager processes and preserve a copy of `state.db` and the named directory before investigating. Do not remove the intent or recursively delete unknown contents to force startup. Any manual repair should preserve the unproven entries outside the named staging location; retry recovery only once that location is absent or verified safe.

## Browser opening

`skillctl ui` always prints `UI available at http://127.0.0.1:<port>` after it binds loopback.

If the browser does not open:

```text
skillctl ui --no-open
```

Then visit the printed URL. On Linux, `xdg-open` must be installed for automatic launch; on macOS, `open` is used. Launch failure never stops the server.

`--port` must be between 0 and 65535. Port `0` asks the kernel for a free port. If the requested port is already bound, the next free loopback port is used. The printed URL is the port that actually bound.
