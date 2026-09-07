# Fetch only the resolved Git commit

Type: task
Status: resolved
Blocked by: 04

## Question

Replace “fetch every branch and tag” with clone/fetch of the commit `ls-remote` already resolved. Keep pinned full SHAs, branches, lightweight tags, and annotated tags working.

## Comments

- Do **not** add `--depth 1` in this ticket. Shallow clones need a separate pinned-SHA study.
- Prefer `git fetch --no-tags --filter=blob:none origin <commit>`.
- First clone may use `--bare --filter=blob:none --single-branch --no-tags`.

## Answer

Dropped `blob:none` and `fetch --prune` of every ref. Clone is `--bare --single-branch --no-tags` (optional `--branch` for a simple ref). Missing commits are fetched with `git fetch --no-tags origin <commit>`. Lazy promisor fetches were the baseline SSL failure; a full single-branch clone of mattpocock/skills is ~3s / 2.3MiB. Existing ref/tag/pinned-SHA tests still pass. No `--depth 1`.
