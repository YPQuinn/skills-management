# Speed up Git Source add and import

Label: wayfinder:map
Status: resolved

## Destination

Make Git `source add` and `skill import` fast enough for a public multi-Skill repository (`https://github.com/mattpocock/skills`, 37 Skills) without weakening the existing safety contract: ambient credentials only, rooted writes, and Import still installing the exact observed complete-tree digest.

Measurements use `HOME=/private/tmp/skillctl-dev-home` and a cold `git-cache`.

## Notes

- Do not skip the Import digest check. Speed comes from fewer git network round-trips and from not re-proving an unchanged commit.
- Ticket 06 (listing-only registration) is the only ticket that may change what `source add` persists. Import and `source check` still take a full observation before Store writes.
- Reuse of a previous Inventory is allowed only when `ls-remote` returns the same commit **and** every Inventory entry already has a complete-tree digest.

## Decisions so far

- [Instrument Git phased timing](issues/01-instrument-phased-timing.md) — `SKILLCTL_GIT_TRACE=1` on stderr; measure script under `scripts/measure.sh`.
- [Skip materialize fetch when the commit is cached](issues/02-dedupe-materialize-fetch.md) — `ensureCommit` is a cache hit when the observed SHA is present; no per-Skill fetch.
- [Reuse Inventory when the Git commit is unchanged](issues/03-reuse-same-commit-inventory.md) — same-commit check is `ls-remote` only; listing-only Inventories are never reused.
- [Batch Git tree listing and blob fetches](issues/04-batch-blob-prefetch.md) — one blob batch per observation; materialize `ls-tree`s one Skill path.
- [Fetch only the resolved Git commit](issues/05-narrow-git-fetch.md) — dropped `blob:none` and all-ref prune; clone `--bare --single-branch --no-tags`; fetch the resolved commit only. No `--depth 1`.
- [Defer complete-tree digest until check or import](issues/06-listing-only-registration.md) — Git `source add` reads `SKILL.md` only; Import/check fill complete-tree digests.

Cold-cache results: [baseline-cold](measurements/baseline-cold/README.md) (add SSL-failed) vs [after-opt](measurements/after-opt/README.md) (add 4.3s, import 37 Skills 7.8s, check 1.3s).

## Not yet specified

- Whether a shallow `--depth 1` clone is worth the pinned-SHA and tag regressions.

## Out of scope

- Changing Local Source observation.
- Concurrent git fetches as the first optimization.
- Replacing the canonical complete-tree digest with a Git tree OID.
