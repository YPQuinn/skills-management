# Publish and maintain a Homebrew Tap

Type: task
Status: resolved
Blocked by: 01

## Goal

Provide a working `brew install YPQuinn/tap/skillctl` path that reuses the existing checksummed GitHub Release archives and remains current without manual checksum copying.

## Acceptance criteria

- `YPQuinn/homebrew-tap` exists as a public GitHub repository.
- Its `skillctl` Formula supports macOS and Linux on amd64 and arm64.
- Formula URLs and SHA-256 values come from a published stable `skills-management` release.
- Homebrew can install and test the Formula on a supported native machine.
- The Tap can regenerate the Formula from the latest release without a cross-repository write token.
- The main README publishes the working Homebrew command.

## Answer

Created the public [`YPQuinn/homebrew-tap`](https://github.com/YPQuinn/homebrew-tap) repository. Its Formula selects the matching one of the four existing v0.1.3 release archives, verifies the published checksum, installs `skillctl`, and tests `skillctl --version`. A daily and manually dispatchable workflow regenerates the Formula from the latest stable release's `SHA256SUMS` using the Tap repository's own token. Native `brew install`, `brew test`, `brew audit --strict`, and `brew style` all pass on macOS arm64.
