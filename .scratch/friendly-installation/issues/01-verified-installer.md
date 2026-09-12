# Implement the verified installer

Type: task
Status: resolved

## Question

Add a portable `install.sh` that selects the matching GitHub Release archive, verifies it, and atomically installs `skillctl` without escalating privileges.

## Acceptance criteria

- POSIX shell syntax works with macOS `/bin/sh` and common Linux shells.
- Darwin/Linux and amd64/arm64 map to existing release archive names.
- Latest and pinned stable versions are supported.
- SHA-256 verification happens before extraction.
- The default destination is `~/.local/bin`; `--install-dir` overrides it.
- Existing installations survive every failure before the final move.

## Answer

Added the POSIX `install.sh`. It selects Darwin/Linux and amd64/arm64 archives, resolves the latest stable version from `SHA256SUMS`, accepts a pinned release and destination override, verifies SHA-256 before extraction, and stages the executable in the destination directory before the final atomic move. It defaults to `~/.local/bin`, never invokes `sudo`, and reports when the directory is absent from `PATH`.
