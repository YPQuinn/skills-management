# Make installation friendly

Label: wayfinder:map
Status: resolved

## Destination

A macOS or Linux user can install `skillctl` from the latest GitHub Release with Homebrew or one portable shell command. Both paths select a supported OS and CPU architecture and verify the published SHA-256 checksum. The shell installer also installs without implicit privilege escalation and supports reproducible version pinning.

## Decisions so far

- Keep GitHub Release archives as the release source; do not introduce a second artifact pipeline.
- Install to `~/.local/bin` by default and never invoke `sudo`.
- Resolve `latest` from the published `SHA256SUMS` so the installer does not depend on `jq` or the GitHub API.
- Keep manual archive installation as the offline and audit-friendly fallback.
- Publish Homebrew through the dedicated public [`YPQuinn/homebrew-tap`](https://github.com/YPQuinn/homebrew-tap) repository now that it exists.
- Let the Tap poll the latest public Release from its own workflow instead of introducing a cross-repository write credential.
- [Verified installer](issues/01-verified-installer.md) — POSIX shell, latest/pinned releases, SHA-256 verification, user-writable atomic destination.
- [Installer verification](issues/02-installer-verification.md) — hermetic `file://` fixtures cover successful and failed installs in the quality gate.
- [Install documentation](issues/03-install-documentation.md) — one-command default plus pinning, inspect-first, PATH, and manual fallback guidance.
- [Homebrew Tap](issues/04-homebrew-tap.md) — a native package-manager path backed by the same four checksummed release archives.

## Delivery order

1. [Implement the verified installer](issues/01-verified-installer.md)
2. [Add hermetic installer verification](issues/02-installer-verification.md)
3. [Rewrite installation documentation](issues/03-install-documentation.md)
4. [Publish and maintain the Homebrew Tap](issues/04-homebrew-tap.md)

## Out of scope
- `.deb`, `.rpm`, Nix, Scoop, or Winget packages.
- `go install`; the module path and generated embedded WebUI currently make that a separate design change.
- Self-update behavior inside `skillctl`.
