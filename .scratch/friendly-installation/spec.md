# Friendly installation

## Problem

The README currently asks users to identify an archive, download two files, choose a checksum command, extract the archive, create an installation directory, and move the binary manually. Archive names also embed the current release version, so the instructions age on every release.

## User contract

The primary package-manager installation command is:

```sh
brew install YPQuinn/tap/skillctl
```

The Tap Formula must select the matching macOS/Linux amd64/arm64 release archive and use its checksum from the published `SHA256SUMS`. The Tap updates itself from the latest stable release without requiring a credential that can write to this repository.

The portable installation command is:

```sh
curl -fsSL https://raw.githubusercontent.com/YPQuinn/skills-management/main/install.sh | sh
```

The installer must:

- support macOS and Linux on amd64 and arm64;
- download only artifacts already published by GitHub Releases;
- verify the selected archive against `SHA256SUMS` before extraction or installation;
- default to the latest stable release;
- accept an explicit stable version and installation directory;
- default to `$HOME/.local/bin` and never call `sudo`;
- preserve an existing installed binary if download, checksum, or extraction fails;
- explain unsupported platforms and a missing PATH entry clearly.

Users who do not want to pipe a remote script into a shell must be shown how to download and inspect it first. Manual archive installation remains documented as a fallback.

## Interface

```text
install.sh [--version vMAJOR.MINOR.PATCH] [--install-dir DIRECTORY]
```

Equivalent environment variables are `SKILLCTL_VERSION` and `SKILLCTL_INSTALL_DIR`. `SKILLCTL_RELEASE_ROOT` is an intentionally undocumented test hook used to run the verification suite without network access.

## Verification

A hermetic shell test builds fake release archives below a temporary `file://` release root and covers:

- latest release installation;
- pinned version installation;
- installation into a path containing spaces;
- checksum rejection without replacing an existing binary;
- invalid arguments and versions.

The test is part of `scripts/check.sh`, while the existing archive and README smoke tests continue to cover the real compiled binary.

The Homebrew Formula is validated with `brew style`, `brew audit --strict`, `brew install`, and `brew test`. Its updater is also exercised through a real manual GitHub Actions dispatch.
