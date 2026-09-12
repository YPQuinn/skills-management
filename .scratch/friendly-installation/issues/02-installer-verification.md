# Add hermetic installer verification

Type: task
Status: resolved
Blocked by: 01

## Question

Exercise installer selection, checksum, destination, and failure behavior without contacting GitHub, then include that suite in the repository check gate.

## Acceptance criteria

- Tests create their own archives and checksums.
- Latest and pinned installs execute the installed fixture.
- A destination containing spaces works.
- A corrupt archive is rejected and cannot replace an existing binary.
- Invalid versions and unknown options fail.

## Answer

Added `scripts/smoke/installer.sh` and wired it into `scripts/check.sh`. The suite creates checksummed `file://` release fixtures, installs latest and pinned versions, exercises a destination containing spaces, proves checksum failure preserves an existing binary, rejects bad arguments, and checks that staging files are cleaned up. It runs without GitHub or any other network service.
