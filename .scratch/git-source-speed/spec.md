# Git Source speed

## Problem

Git `source add` and `skill import` are slow on a real multi-Skill repository. The current path is correctness-first:

1. `clone --bare --filter=blob:none` then `fetch --prune` of **every** branch and tag.
2. Observation downloads **every blob of every Skill** to compute complete-tree digests.
3. Import always re-observes the whole Source, then `ensureCache` (another full fetch) **per Skill** before materializing.

`https://github.com/mattpocock/skills` exposes 37 Skills. That turns the per-Skill fetch into 1 + 37 network refreshes on import.

## What must stay true

- Registration still reaches the Source or it is not saved.
- Import still materializes the complete Skill tree and verifies the canonical digest.
- Unsafe nodes (escaping symlink, gitlink, LFS pointer, special files) still reject the Skill.
- Tokens never enter locators, argv, logs, or cache keys.
- `file://` hermetic Git tests keep passing, including branch/tag/pinned-SHA updates.

## Tickets

| NN | Change |
| --- | --- |
| 01 | Phased timing (`SKILLCTL_GIT_TRACE`) and a cold-cache measurement recipe |
| 02 | Materialize does not refresh remotes when the observed commit is already in cache |
| 03 | Same-commit Import/Check reuses a complete Inventory; only `ls-remote` hits the network |
| 04 | One `ls-tree` + one blob batch per observation, not per Skill |
| 05 | Clone/fetch only the resolved commit, not every ref |
| 06 | `source add` lists Skills from `SKILL.md` only; complete-tree digest waits for check/import |

## Measurement

Home: `/private/tmp/skillctl-dev-home`. Remote: `https://github.com/mattpocock/skills`.

Each measured run **deletes** `$HOME/.skillctl/git-cache` (and for add+import, resets the installation) so the Git cache cannot hide a previous fetch.
