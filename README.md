# Skill Manager

Skill Manager is a local-first tool for macOS and Linux. It keeps an authoritative **Skill Store** of [Agent Skills](https://agentskills.io/specification), imports them from local directories or Git remotes, and projects selected Skills into Agent **Targets** as absolute symlinks.

The CLI is `skillctl`. The same binary embeds a localhost WebUI.

## Support

| Platform | Archive |
| --- | --- |
| macOS Apple silicon | `skillctl_v0.1.2_darwin_arm64.tar.gz` |
| macOS Intel | `skillctl_v0.1.2_darwin_amd64.tar.gz` |
| Linux amd64 | `skillctl_v0.1.2_linux_amd64.tar.gz` |
| Linux arm64 | `skillctl_v0.1.2_linux_arm64.tar.gz` |

Windows, 32-bit systems, and other Unix targets are unsupported. Each archive is a single statically linked `skillctl` binary (`CGO_ENABLED=0`). The Linux binary has no system SQLite or C toolchain dependency.

Local Sources need no extra tools. Git Sources require `git` on `PATH`. `gh` is optional: Skill Manager uses whatever ambient Git/SSH/`gh` credentials already work on the machine and never stores tokens.

## Install

Download the archive for this machine and `SHA256SUMS` from the release. Verify **that archive only** (the checksum file lists all four), then extract `skillctl` onto `PATH`.

Linux (amd64 example):

```text
grep 'skillctl_v0.1.2_linux_amd64.tar.gz$' SHA256SUMS | sha256sum -c -
tar -xzf skillctl_v0.1.2_linux_amd64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 skillctl ~/.local/bin/skillctl
```

macOS (Apple silicon example):

```text
grep 'skillctl_v0.1.2_darwin_arm64.tar.gz$' SHA256SUMS | shasum -a 256 -c -
tar -xzf skillctl_v0.1.2_darwin_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 skillctl ~/.local/bin/skillctl
```

Replace the archive filename with the row in the support table that matches this OS and architecture.

Source builds are a developer path and need Go, Node, and npm:

```text
./scripts/build.sh
```

That writes `dist/skillctl`. Homebrew, system packages, `go install`, curl installers, and self-update are not provided.

## Initialize and CLI workflow

`skillctl init` writes `~/.skillctl/config.toml`, creates `~/.skillctl/state.db`, and creates the Skill Store at `~/.skillctl/store`. Pass `--store /absolute/path` to put the Store somewhere else. Initialization is one-shot; a second `init` refuses.

If configuration exists but `state.db` is missing, ordinary commands report `state_missing` and do not invent empty state. Recover Store directories as unbound Skills with `skillctl init --recover-store` (see [Troubleshooting](docs/troubleshooting.md)).

Registration never imports, and Assignment never writes Target files. Import, Synchronization, and Distribution are explicit. From a directory that contains `./my-skills/demo-skill/SKILL.md`:

```bash
skillctl --version
skillctl init
skillctl status
skillctl source add ./my-skills --name local
skillctl source show local
skillctl skill import --source local --path demo-skill
skillctl group create demo
skillctl group add-skill demo demo-skill
skillctl target add --path ~/agent-skills --name demo-target
skillctl target assign demo-target --group demo
skillctl target status demo-target --refresh
skillctl target distribute demo-target --dry-run
skillctl target distribute demo-target
```

`source add` accepts a local directory, an HTTPS/SSH Git URL, or GitHub `owner/repo` shorthand. Git refs use `--ref`; a subdirectory of a Git Source uses `--subpath`. `skill import` also accepts `--skill <name>` or `--all`.

Built-in Target adapters: `universal`, `claude-code`, `codex`, `cursor`, `gemini-cli`, `opencode`, `pi`, `github-copilot`. Example: `skillctl target add --adapter claude-code --scope user`.

`--json` prints exactly one JSON value on stdout. Help and `--version` exit 0.

## Default paths

| Path | Role |
| --- | --- |
| `~/.skillctl/config.toml` | Installation config (`store_path`, `state_db_path`) |
| `~/.skillctl/state.db` | SQLite state |
| `~/.skillctl/store` | Skill Store (unless `--store` was used) |
| `~/.skillctl/skillctl.lock` | Installation writer lock |
| `~/.skillctl/locks/` | Store and Source locks |

All writes stay under these chosen paths plus registered Target containers.

## WebUI

```text
skillctl ui
```

Listens on `http://127.0.0.1:10000` and tries to open a browser. If that port is in use, the next free port is used. `--port 0` picks a free port. `--no-open` never launches a browser. If the browser cannot be opened, the URL is still printed. The server is loopback-only.

## Uninstall

Distribution is the only way Skill Manager removes **Managed Links**. Unassign and redistribute first if those Target entries should go:

```text
skillctl target unassign demo-target --group demo
skillctl target distribute demo-target
```

Then remove the binary and the installation directory:

```text
rm "$(command -v skillctl)"
rm -rf ~/.skillctl
```

Deleting `~/.skillctl` does not walk Targets. Leftover Managed Links become ordinary unmanaged symlinks. Unmanaged Target files are never deleted. Skill Manager never deletes a Target container.

## Release validation

Maintainers run `./scripts/release.sh v0.1.2` from a clean checkout at that exact tag. It runs the quality and README gates, then produces four archives and `SHA256SUMS`.

Pushing a version tag also runs the **release archives** workflow: it packages once, then downloads and verifies those same archives on four native runners without rebuilding. Only after all jobs pass should the `release-archives` artifact's four archives and checksum file be published. The workflow has read-only repository permissions and does not create a GitHub Release. A manual rerun must select the version tag, not a branch.

## Safety and troubleshooting

- [Safety](docs/safety.md): Source versus Store, Synchronization versus Distribution, unmanaged content, conflicts, Adopt, snapshots, and rollback.
- [Troubleshooting](docs/troubleshooting.md): credentials, unavailable Sources, Target conflicts, broken links, locks, recovery, and browser launch.
- `skillctl <command> --help` for the live flag contract, especially `init --recover-store`, delete, adopt, distribute, sync, keep-store, accept-source, and rollback.
