# Rewrite installation documentation

Type: task
Status: resolved
Blocked by: 02

## Question

Make the verified installer the shortest path in README while retaining inspect-first and manual installation paths.

## Acceptance criteria

- The default command does not contain a release version.
- Version pinning and custom installation directories are shown.
- Users can inspect the installer before running it.
- Manual checksummed installation remains available without hard-coded archive versions.
- Unsupported platforms and PATH expectations remain explicit.

## Answer

Reworked README's Install section around one version-independent command. Added version pinning, a custom user-writable destination, an inspect-before-run path, PATH guidance, generic archive patterns, and retained checksummed manual installation for users who do not want the installer.
