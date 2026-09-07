# Instrument Git phased timing

Type: task
Status: resolved
Blocked by:

## Question

Add an opt-in Git phase trace so add/import/check can be split into resolve / cache / discover / materialize, with git invocation counts, without changing default CLI or `--json` output.

## Comments

- Enable with `SKILLCTL_GIT_TRACE=1`. Lines go to stderr as `skillctl-git: ...`.
- Default runs and `--json` stdout must stay unchanged.

## Answer

`SKILLCTL_GIT_TRACE=1` writes phase lines (`resolve_commit`, `clone`, `ensure_commit`, `discover`, `cache hit`, `fetch`) and every `runGit` invocation to stderr. Credential helper argv is redacted. `--json` stdout is unchanged. Recipe: `.scratch/git-source-speed/scripts/measure.sh`.
