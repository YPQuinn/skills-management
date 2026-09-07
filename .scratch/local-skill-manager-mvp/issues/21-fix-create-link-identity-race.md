# Preserve external replacements before create identity sampling

Type: task
Status: resolved

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

The original failing reproduction is retained in the patch against `db09150`. The implementation follow-up promotes the regression into the test suite; do not re-apply the historical patch over the fixed implementation.

## Acceptance

- Bind creation ownership to the object actually created, not an arbitrary subsequent occupant of the visible slug.
- Preserve no-overwrite behavior and crash recovery; do not add an implicit adoption fallback.
- Promote the reproduction into a regression at the actual create boundary and show it green after the fix.
- Re-run creation/replacement/crash/recovery tests and four-platform acceptance on the updated candidate.
- Revisit ticket 06's creation representation if closing the race needs a staging/installation step; document any contract change explicitly.

## Implementation follow-up

Creation now stages a symlink under a random 0700 directory, samples the identity there, persists it through the application's intent callback, and only then publishes with a no-overwrite rename. The live slug is checked against that pre-publication proof, never used to obtain a new identity. The existing `slot_name` and identity fields suffice; no schema migration was added. Create and remove reuse the pinned-directory and no-overwrite primitives.

Create recovery cleans only staging that matches its recorded proof (or an empty directory), then checks the live slug. Unproven/replaced staging and its receipt are preserved with `recovery_failed`. A crash before publication with proven staging rolls back that staging and leaves the desired link missing for a fresh explicit retry. A crash after publication finalizes only the matching original link. The missing-Target recovery regression also exposed and fixed `os.IsNotExist` failing to recognize a wrapped missing-path error; `errors.Is` now recognizes it.

Ticket 06 and the safety/troubleshooting guides record the staging-contract amendment, conservative recovery behavior, and the remaining same-UID limitation. A private directory is not claimed to exclude a malicious process running as the same OS user.

Local validation passed:

- Original create-before-live-sample regression: three repetitions, now green.
- New persistence-before-publication, failed-persistence, concurrent-final-entry, replaced/unknown/occupied-staging tests.
- Process crashes before staging, before proof persistence, after proof persistence, and after publication; replaced-staging recovery and actual app intent persistence.
- Focused race-detector regression suite: three repetitions, passed.
- `./scripts/check.sh`: passed, including 144 frontend tests and all Go tests.
- Embedded Chromium/Firefox/WebKit acceptance: 8/8 passed; README quickstart and native macOS arm64 archive smoke passed.

These local results describe the changed working tree, not the prior binary's embedded Git SHA.

## Answer

Resolved by `dff5265bc5b0c8cf1b4bf9fa01726ae3dee1dbb7`. Both [branch acceptance 34085004614](https://github.com/YPQuinn/skills-management/actions/runs/34085004614) and [PR merge-ref acceptance 34085006535](https://github.com/YPQuinn/skills-management/actions/runs/34085006535) passed all four native platform jobs. Linux amd64 passed the full race suite and browser acceptance; macOS arm64 passed full functional/browser acceptance; macOS amd64 and Linux arm64 passed native archive smoke. The original deterministic regression is retained in the test suite and passes.

The cause was sampling ownership from a public path after creating it. Pre-publication staging plus durable identity persistence removes that public-slug sampling window; explicit unknown-staging recovery and the same-UID limitation are documented. Ticket 19 still owns final merged/tagged-candidate and versioned-archive release acceptance.
