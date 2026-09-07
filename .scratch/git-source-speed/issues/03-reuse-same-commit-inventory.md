# Reuse Inventory when the Git commit is unchanged

Type: task
Status: resolved
Blocked by: 02

## Question

After `ls-remote` resolves the same commit as `LastCommit`, and every Inventory entry already has a complete-tree digest, skip clone/fetch/discover and reuse the persisted Inventory.

## Comments

- Empty digests (listing-only registration) must **not** be reused; Import then takes a full observation.
- Still update `LastCheckedAt` so a cheap check is recorded.
- A moved default branch must still be observed.

## Answer

`reuseGitObservation` runs before Observe. Listing-only Inventories (empty digests) are never reused. Same-commit check against mattpocock/skills is one `ls-remote` (1.27s). `TestGitAddListsThenImportObservesOnceThenReuses` and `TestReuseGitObservationRequiresCompleteDigests` lock the contract.
