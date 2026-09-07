# Skip materialize fetch when the commit is cached

Type: task
Status: resolved
Blocked by: 01

## Question

Stop calling `fetch --prune` of all refs inside each Skill materialization. If the observed commit is already in the per-location cache, materialize only reads local git objects.

## Comments

- Observe may still refresh when it needs a newly resolved commit.
- A missing commit still fetches that commit (not every branch and tag).

## Answer

`ensureCommit` returns immediately when `rev-parse <commit>^{commit}` succeeds. Materialize after add/import observation is cache-hit only. `TestMaterializeDoesNotFetchWhenCommitCached` locks this. After-opt import of 37 Skills: 37 cache hits, zero `fetch`/`clone`.
