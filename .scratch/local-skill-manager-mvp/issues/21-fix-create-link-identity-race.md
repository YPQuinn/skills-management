# Preserve external replacements before create identity sampling

Type: task
Status: open

## Question

Close the remaining create-time ownership race before merging PR #1 or releasing v0.1.1. Ticket 06 requires that a replaced Target entry never becomes owned implicitly or gets deleted through an old ownership claim.

## Evidence

Reviewed `origin/main` (`e0d6326`) through candidate `db09150`. In `internal/distribution/mutate.go:48-62`, `CreateLink` calls `symlinkat`, then independently stats the visible slug. A same-raw external replacement between these calls supplies the identity returned as the newly created link's proof. Later `VerifyLink` accepts that proof and `RemoveManagedLink` deletes the replacement. This unsafe creation window existed before the branch's identity fix and remains unclosed; the new identity proof can still certify the wrong object.

A deterministic test-only hook between creation and sampling simulates a valid external rename. No syscall results or identity values are mocked. Existing create-replacement tests use an app hook after the identity has already been sampled and persisted, so they miss this earlier window.

Reproduction patch: [create-link-identity-race.patch](../assets/create-link-identity-race.patch). Against `db09150`:

```bash
git apply .scratch/local-skill-manager-mvp/assets/create-link-identity-race.patch
go test ./internal/distribution -run '^TestCreateIdentityDoesNotClaimPreSampleReplacement$' -count=3
```

All three runs failed:

```text
created proof incorrectly authorizes an externally replaced symlink
external replacement deleted: remove result=0, error=<nil>
```

The test and temporary hook are retained in the patch, not installed in production code. `git apply --check` passed after removing the temporary instrumentation from the checkout. This is an unresolved release blocker, not a passing regression.

## Acceptance

- Bind creation ownership to the object actually created, not an arbitrary subsequent occupant of the visible slug.
- Preserve no-overwrite behavior and crash recovery; do not add an implicit adoption fallback.
- Promote the reproduction into a regression at the actual create boundary and show it green after the fix.
- Re-run creation/replacement/crash/recovery tests and four-platform acceptance on the updated candidate.
- Revisit ticket 06's creation representation if closing the race needs a staging/installation step; document any contract change explicitly.
