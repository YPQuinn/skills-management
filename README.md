<div align="center">

<img src="./internal/webui/public/favicon.svg" alt="Skill Manager logo" width="72" height="72">

# Skill Manager

**Keep one authoritative Agent Skill library and distribute it safely to every local coding agent.**

[![Acceptance](https://github.com/YPQuinn/skills-management/actions/workflows/acceptance.yml/badge.svg)](https://github.com/YPQuinn/skills-management/actions/workflows/acceptance.yml)
[![Latest release](https://img.shields.io/github/v/release/YPQuinn/skills-management?style=flat-square)](https://github.com/YPQuinn/skills-management/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-555?style=flat-square)](#supported-platforms)

[Overview](#overview) · [Install](#install) · [Quick start](#quick-start) · [WebUI](#webui) · [Safety](#safety-model) · [Development](#development)

</div>

Skill Manager is a local-first CLI and WebUI for managing [Agent Skills](https://agentskills.io/specification). It imports Skills from local directories or Git repositories into a central **Skill Store**, organizes them into reusable Groups, and distributes them to Agent Targets as absolute symlinks.

The CLI is `skillctl`. The same statically linked binary embeds the WebUI and uses a local SQLite database—there is no remote service or account to configure.

> [!IMPORTANT]
> Registering a Source does not import its Skills, assigning a Skill does not write to a Target, and synchronization never runs in the background. Import, synchronization, and Distribution are explicit operations.

## Overview

```text
Local directories ─┐
                   ├─> Sources ── import / sync ──> Skill Store ── distribute ──> Agent Targets
Git repositories ──┘                                  │
                                                      └─> Groups and Assignments
```

The Skill Store is the authoritative copy. Sources provide upstream content; Targets receive managed links back to the Store.

### Features

- **Local and Git Sources** — scan directories, HTTPS/SSH repositories, GitHub `owner/repo` shorthand, branches, tags, commits, and repository subdirectories.
- **Explicit synchronization** — compare Source, Store, and the accepted baseline before changing content; inspect diffs and resolve divergent changes deliberately.
- **Groups and Assignments** — compose Skills once and assign a Skill or Group to multiple Targets.
- **Safe Distribution** — preview changes, create links atomically, never overwrite unmanaged entries, and remove only links whose ownership can still be proven.
- **Built-in Agent adapters** — resolve user- or project-level Skill locations for common coding agents, with custom directory Targets when needed.
- **CLI and local WebUI** — use either interface against the same local state and safety rules.
- **Automation-friendly output** — command-level `--json` emits exactly one JSON value on stdout.

## Supported Agents

| Adapter | Agent |
| --- | --- |
| `universal` | Universal `.agents/skills` layout |
| `claude-code` | Claude Code |
| `codex` | Codex |
| `cursor` | Cursor |
| `gemini-cli` | Gemini CLI |
| `opencode` | OpenCode |
| `pi` | Pi |
| `github-copilot` | GitHub Copilot |

Adapters discover conventional user and project Skill directories. They do not take ownership of an Agent installation, and detection is advisory. You can always register an explicit directory with `--path`.

## Install

Download the archive for your machine and `SHA256SUMS` from the [latest release](https://github.com/YPQuinn/skills-management/releases/latest).

### Supported platforms

| Platform | Release archive |
| --- | --- |
| macOS Apple silicon | `skillctl_v0.1.3_darwin_arm64.tar.gz` |
| macOS Intel | `skillctl_v0.1.3_darwin_amd64.tar.gz` |
| Linux amd64 | `skillctl_v0.1.3_linux_amd64.tar.gz` |
| Linux arm64 | `skillctl_v0.1.3_linux_arm64.tar.gz` |

Verify the downloaded archive, extract it, and place `skillctl` on `PATH`. For example, on macOS Apple silicon:

```bash
grep 'skillctl_v0.1.3_darwin_arm64.tar.gz$' SHA256SUMS | shasum -a 256 -c -
tar -xzf skillctl_v0.1.3_darwin_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 skillctl ~/.local/bin/skillctl
skillctl --version
```

On Linux, use `sha256sum -c -` instead of `shasum -a 256 -c -`.

> [!NOTE]
> Windows, 32-bit systems, and other Unix targets are not supported. Release binaries are statically linked with `CGO_ENABLED=0`; the Linux build does not require a system SQLite library or C toolchain.

Local Sources require no additional tools. Git Sources require `git` on `PATH` and reuse your existing Git, SSH, or optional GitHub CLI credentials. Skill Manager never stores tokens.

## Quick start

Assume `./my-skills/demo-skill/SKILL.md` contains a valid Agent Skill and `~/agent-skills` is the directory you want to manage.

```bash
skillctl init
skillctl source add ./my-skills --name local
skillctl skill import --source local --path demo-skill
skillctl group create demo
skillctl group add-skill demo demo-skill
skillctl target add --path ~/agent-skills --name demo-target
skillctl target assign demo-target --group demo
skillctl target status demo-target --refresh
skillctl target distribute demo-target --dry-run
skillctl target distribute demo-target
```

The final command creates `~/agent-skills/demo-skill` as a managed absolute symlink to the Skill Store. Re-running Distribution reconciles the Target with its current Assignments.

To use a built-in adapter instead of a custom path:

```text
skillctl target add --adapter claude-code --scope user --name claude-user
skillctl target add --adapter cursor --scope project --project /path/to/project --name cursor-project
```

## WebUI

Start the embedded WebUI with:

```text
skillctl ui
```

Skill Manager listens on `http://127.0.0.1:10000` and opens a browser. If the port is occupied, it tries the next available loopback port; `--port 0` asks the operating system for a free port, and `--no-open` skips browser launch.

The WebUI manages Sources, Skills, synchronization, Groups, Assignments, Targets, and Distribution using the same local state as the CLI.

## Core workflows

### Import from a Git Source

A Git Source may be an HTTPS/SSH URL or GitHub repository shorthand. `--ref` selects a branch, tag, or commit; `--subpath` limits inventory scanning to one repository directory.

```text
skillctl source add acme/agent-skills --name team --ref main --subpath skills
skillctl source show team
skillctl skill import --source team --all
```

Source registration scans inventory but never imports automatically.

### Synchronize safely

Synchronization retrieves changes in one direction: **Source → Skill Store**.

```text
skillctl skill check demo-skill
skillctl skill diff demo-skill
skillctl skill sync demo-skill
```

`sync` automatically applies only `source_changed`. If Store content changed locally or both sides diverged, choose an explicit resolution:

| Action | Command | Result |
| --- | --- | --- |
| Keep Store | `skillctl skill keep-store demo-skill` | Preserve Store bytes and accept the current Source as the new baseline |
| Accept Source | `skillctl skill accept-source demo-skill` | Snapshot the Store, then replace it with validated Source content |
| Detach | `skillctl skill detach demo-skill` | Remove the Source Binding and keep the Store copy |
| Roll back | `skillctl skill rollback demo-skill` | Restore the previous Store snapshot |

Use `skillctl source sync <source>` to check a Source once and synchronize all bound Skills independently.

### Distribute to an Agent Target

Assignments describe desired state. Distribution is the only operation that reconciles that state on disk.

```text
skillctl target assign claude-user --skill demo-skill
skillctl target distribute claude-user --dry-run
skillctl target distribute claude-user
skillctl target status claude-user --refresh
```

## Safety model

Skill Manager is conservative around user files and interrupted operations:

- It never overwrites an existing unmanaged file, directory, or foreign symlink.
- It never adopts an existing symlink implicitly—even if that link already points to the correct Skill.
- It removes only Managed Links whose destination and physical identity still match the ownership record.
- It never deletes a Target container or enumerates unrelated Target entries for cleanup.
- Store replacements retain one previous snapshot for one-step rollback.
- Source outages preserve the last successful inventory, bindings, Groups, Assignments, and Store content.
- Concurrent writers fail immediately instead of waiting or guessing.

An eligible existing symlink can be adopted explicitly:

```text
skillctl target adopt <target> <skill>
```

For the full ownership, conflict, recovery, and rollback contract, read [Safety](docs/safety.md). Operational fixes are documented in [Troubleshooting](docs/troubleshooting.md).

## Data locations

By default, `skillctl init` creates:

| Path | Purpose |
| --- | --- |
| `~/.skillctl/config.toml` | Installation configuration |
| `~/.skillctl/state.db` | Embedded SQLite state |
| `~/.skillctl/store` | Authoritative Skill Store |
| `~/.skillctl/skillctl.lock` | Installation writer lock |
| `~/.skillctl/locks/` | Store and Source locks |

Use `skillctl init --store /absolute/path` to choose another Store location. Initialization is one-shot. If configuration survives but `state.db` is missing, recover valid Store directories as unbound Skills with `skillctl init --recover-store`.

## CLI reference

```text
skillctl source --help    # register, inspect, check, sync, and delete Sources
skillctl skill --help     # import, inspect, sync, resolve, roll back, and delete Skills
skillctl group --help     # create Groups and manage membership
skillctl target --help    # register, assign, inspect, distribute, adopt, and delete Targets
skillctl ui --help        # configure and start the local WebUI
```

Successful human-readable output goes to stdout and diagnostics go to stderr. JSON failures emit one JSON error value on stdout. Exit code `1` indicates a blocked or failed operation; exit code `2` indicates invalid arguments or configuration.

## Development

Source builds require Go 1.26.4, Node.js 22, pnpm 12.4.1, and Git.

```bash
git clone https://github.com/YPQuinn/skills-management.git
cd skills-management
./scripts/build.sh
```

The development binary is written to `dist/skillctl`. Run the complete TypeScript, Oxlint, Vitest, Go formatting, vet, and test gate with:

```bash
./scripts/check.sh
```

## Uninstall

> [!WARNING]
> Distribution is the only operation that removes Managed Links. Unassign and redistribute before uninstalling if Target links should be removed.

```text
skillctl target unassign demo-target --group demo
skillctl target distribute demo-target
rm "$(command -v skillctl)"
rm -rf ~/.skillctl
```

Deleting `~/.skillctl` does not walk Target directories. Any leftover links become unmanaged symlinks, while unrelated Target content remains untouched.

## Documentation

- [Agent Skills specification](https://agentskills.io/specification)
- [Safety guarantees and conflict handling](docs/safety.md)
- [Troubleshooting and recovery](docs/troubleshooting.md)
- Run `skillctl <command> --help` for the live CLI contract.
