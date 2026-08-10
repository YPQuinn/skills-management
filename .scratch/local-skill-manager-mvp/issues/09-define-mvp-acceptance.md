# Define MVP packaging and acceptance

Type: grilling
Status: resolved
Blocked by: 08

## Question

What installation path, supported builds, automated checks, and end-to-end CLI and WebUI scenarios must pass to establish that the single-executable macOS/Linux MVP has reached the destination?

## Answer

### Installation and supported artifacts

The formal MVP installation path is a precompiled single-binary archive. A user downloads the archive for their platform, extracts `skillctl`, installs it into `~/.local/bin` or another directory on `PATH`, and runs `skillctl init`. Source builds are a developer path requiring Go, Node, and npm. Homebrew, system packages, `go install`, curl installers, self-update, and desktop installers are not part of MVP acceptance.

The release publishes four `CGO_ENABLED=0` artifacts:

```text
skillctl_<version>_darwin_arm64.tar.gz
skillctl_<version>_darwin_amd64.tar.gz
skillctl_<version>_linux_arm64.tar.gz
skillctl_<version>_linux_amd64.tar.gz
SHA256SUMS
```

Windows, 32-bit systems, and other Unix targets are unsupported. The Linux binary has no system SQLite or C toolchain dependency. Local Sources require no external executable. Git Sources require `git`; `gh` remains an optional ambient-authentication convenience. Failure to open a browser prints the URL, and `--no-open` is always supported.

Versions use SemVer, with the first MVP release designated `v0.1.0`. `skillctl --version` reports the version and Git commit but not a build timestamp. The release build requires a clean worktree and a tag matching the injected version. `SHA256SUMS` covers the archives. Signing, SBOMs, SLSA provenance, notarization, and update metadata are not required for this MVP.

Every advertised OS/architecture combination must execute its archive on a native CI runner. If no native runner is available, that artifact is not formally released. Each platform smoke extracts the archive, checks `--version`, initializes an isolated HOME, starts `skillctl ui --no-open`, requests the API and a SPA deep link, and shuts down cleanly. The complete functional suites run on at least macOS arm64 and Linux amd64; the other two native platforms run the release smoke. `go test -race ./...` runs at least on Linux amd64.

### Build and repository contract

Vite `dist/` is generated and not committed. `package-lock.json` is committed and all deterministic checks use `npm ci`. A missing or failed frontend build must prevent Go release compilation; the repository does not carry a placeholder SPA or fallback UI merely to make a bare `go build` succeed.

The repository exposes only these top-level POSIX build entry points:

- `./scripts/build.sh` — install locked frontend dependencies, build the Appica SPA, and build the native development binary at `dist/skillctl`.
- `./scripts/check.sh` — run all stable static, unit, and integration checks.
- `./scripts/release.sh v0.1.0` — validate tag and worktree, run checks and end-to-end acceptance, then produce the four archives and checksum file without uploading or creating a tag.

There is no additional Make, Task, Just, or GoReleaser entry point. Go test packages and Playwright configuration remain the real test runners rather than being wrapped in another custom framework.

The release smoke proves the binary is self-contained by extracting only `skillctl` into an otherwise empty, read-only working directory with no Node, `node_modules`, Vite output, migration files, or templates. Initialization, embedded SQLite migrations, `skillctl ui`, `index.html`, hashed assets, client deep links, and `/api/v1` must work. All writes must remain within explicitly selected temporary configuration, Store, state, and Target paths. Linux additionally verifies that the binary has no CGO/system-SQLite dynamic dependency.

### Automated quality gate

`./scripts/check.sh` runs, in order:

1. `npm ci`;
2. TypeScript type checking;
3. frontend ESLint;
4. frontend Vitest;
5. Vite production build;
6. `gofmt` difference checking;
7. `go vet ./...`;
8. `go test ./...`.

Tests use temporary directories, a temporary HOME, temporary SQLite databases, and local Git fixtures. They never touch real Agent configuration or rely on the network. There is no coverage-percentage gate, `npm audit`, `govulncheck`, or unstable online scanner in release acceptance.

Handler contract tests cover every REST route and method, HTTP status mapping, JSON error code, Content-Type enforcement, Host and Origin rejection, API 404 behavior, and API/SPA separation. `/api/v1` is versioned for the embedded WebUI but is not declared a public third-party API. MVP acceptance does not require OpenAPI, generated clients, or a separate API documentation site. Frontend request types and server DTOs are kept aligned through representative contract fixtures.

CLI exit codes form part of the contract:

- `0` means the command succeeded; a read-only command may report conflicts, staleness, or attention while still succeeding.
- `1` means an attempted use case was blocked, partial, or failed. Any blocked/failed item makes a batch exit `1` while retaining the complete result.
- `2` means command, argument, configuration, or input validation failed before the operation began.

Help and version return `0`. Human output uses stdout, diagnostics use stderr, and `--json` stdout is always exactly one valid JSON value.

### CLI end-to-end acceptance

The primary CLI journey runs a built release candidate in an isolated temporary HOME and performs the following through separate processes:

1. initialize a custom Skill Store;
2. register a multi-Skill Local Source and prove registration does not import;
3. check Source Inventory and explicitly import one Skill;
4. create a Group and add the Skill;
5. register a temporary custom Target and assign the Group;
6. refresh status and observe the desired Skill as missing;
7. run Distribution dry-run and Distribution, then verify the real absolute symlink;
8. modify Source content, observe `source_changed`, synchronize, and verify Store content and the existing Target link;
9. remove the Assignment, redistribute, and prove only the Managed Link is removed;
10. reopen configuration and SQLite in a new process and verify durable state and parseable JSON output.

A separate adversarial journey proves the safety boundary:

- an unmanaged same-name file produces a Target conflict and remains byte-for-byte unchanged;
- an unmanaged symlink to the correct Store Skill is not adopted implicitly, but explicit Adopt establishes ownership;
- simultaneous Source and Store changes produce a synchronization conflict that ordinary and batch sync skip;
- Keep Store leaves content untouched, while explicit Accept Source creates the previous snapshot;
- rollback restores content without changing Binding, Group, or Assignment relationships;
- an unavailable Source preserves stale prior Inventory and relationship state;
- referenced-Skill deletion is blocked by default, and explicit cleanup never removes a replacement that can no longer be proven managed.

Human and JSON output each cover the key status and error shapes without duplicating the entire scenario matrix.

### Recovery and concurrency acceptance

Crash recovery is tested at process level, not only as planner output. A test helper child process terminates at test-only commit hooks. Store replacement covers at least intent-before-filesystem, old-content-in-recovery, new-content-installed-before-SQLite-commit, and SQLite-committed-before-cleanup phases. Distribution covers create and remove after filesystem mutation but before Managed Link finalization. A fresh process then opens Skill Manager and must recover live content, SQLite state, snapshots, and link ownership deterministically without losing candidate content. Failpoints are test-only and are not published as environment variables or CLI switches.

Deterministic cross-process tests prove that:

- a second Store writer receives an explicit lock error while an exclusive Store lock is held and succeeds after retry;
- simultaneous writers to one Target do not interleave;
- different Targets can reconcile concurrently under a Store shared lock;
- replacing a Target entry between inspection and mutation is detected and external content is preserved;
- concurrent CLI and REST mutations either serialize or fail explicitly, never partially commit;
- finite timeouts expose deadlocks without random sleeps.

If configuration exists but `state.db` is missing, ordinary commands return `state_missing` rather than silently creating empty state. Explicit `skillctl init --recover-store` and the WebUI setup recover valid top-level Store directories as newly identified unbound Skills using directory names as slugs. Sources, Groups, Assignments, Target registrations, and Managed Link ownership are reported as unrecoverable; existing Target links remain unmanaged. Unprovable internal Baseline/snapshot trees are preserved and reported rather than guessed or deleted. An end-to-end test proves no Store content is rewritten during recovery.

### WebUI end-to-end acceptance

Playwright runs against the SPA embedded in the built `skillctl`, never against Vite dev server. Chromium runs the complete journey; Firefox and WebKit each run startup, navigation, resource reading, and one safe mutation smoke. The MVP supports current evergreen Chromium, Firefox, and Safari rather than legacy browser versions. Tests locate controls by accessible roles, labels, and state—not Tailwind classes or Appica internal DOM.

The Chromium journey uses only WebUI actions to:

1. initialize from `/setup` into a temporary Store;
2. verify the empty `/skills` state;
3. add a Local Source, inspect Inventory, and explicitly import a Skill;
4. navigate among Source, Skill, and Group resources, create a Group, and add the Skill;
5. create a custom Target, add a Group Assignment, and inspect the expanded desired set;
6. preview and run Distribution and verify the real symlink externally;
7. modify Source content, refresh, view the Skill diff, and synchronize;
8. create a Target conflict and prove only legal non-overwriting actions appear;
9. refresh deep links such as `/skills/<slug>?tab=synchronization` and `/targets/<name>`, and use browser back/forward;
10. restart `skillctl ui` and prove the rendered state comes from SQLite.

Important UI claims are cross-checked against REST or the filesystem instead of asserting only visible text.

Axe runs on setup, all four resource lists, and representative detail pages with no critical or serious violations. Keyboard-only tests complete Add Source, Import, Assignment, Distribution, and destructive confirmation; overlays receive and restore focus correctly. Desktop tests prove the three-column master-detail shape, while narrow viewport tests prove list/detail route separation and no horizontal overflow. Theme tests cover system default, light/dark selection, and refresh persistence. Reduced-motion preference must suppress unnecessary animation. Full-page pixel snapshots are not a release gate.

### GitHub and onboarding acceptance

Automated Git tests use local bare remotes to cover default branches, branches, tags, pinned commits, updates, and unavailability. GitHub shorthand, SSH/HTTPS normalization, and `git`/`gh` invocation are tested with controlled fake executables. CI never depends on GitHub availability or credentials. Before declaring the destination reached, a human performs one public GitHub smoke that registers a real public repository, discovers and imports a Skill, and checks it again. Private-repository acceptance is limited to proving that existing Git/`gh` credentials are invoked and tokens are never persisted; no private release fixture is required.

The finished repository includes:

- a root `README.md` covering support, download and installation, initialization, CLI/WebUI quickstarts, default paths, Git dependency, and uninstall behavior;
- `docs/safety.md` explaining Source versus Store, Synchronization versus Distribution, unmanaged-content protection, conflicts, Adopt, snapshots, and rollback;
- `docs/troubleshooting.md` covering credentials, Source unavailability, Target conflicts, broken links, lock/recovery failures, and browser-opening failures;
- accurate Cobra help and examples, especially for consequential operations.

README quickstart commands execute as an automated smoke so they cannot silently drift. A separate documentation site, man pages, video tutorial, and shell-completion tutorial are outside MVP acceptance.

### Destination gate

The destination is reached only when `check.sh`, all required native artifact smokes, the complete CLI and WebUI suites, crash/concurrency recovery tests, Store recovery test, and the public GitHub smoke pass against the same release candidate; the release archives and checksums are produced; and the onboarding documentation describes that candidate. Compilation alone, a Vite-only prototype, or a manually exercised development checkout does not satisfy the map.
