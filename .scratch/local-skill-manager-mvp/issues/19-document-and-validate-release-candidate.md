# Document and validate the MVP release candidate

Type: task
Status: claimed
Blocked by: 18, 21

## Question

Finish the destination gate against one release candidate: write and verify the README, safety guide, troubleshooting guide, and Cobra help; run the complete check, native archive, CLI, REST, WebUI, recovery, concurrency, and Store-recovery gates; perform the public GitHub Source smoke; resolve release-blocking findings; and produce the final `v0.1.1` archives and checksums with recorded validation results.

## Comments

### CI follow-up on `fix/linux-ci-link-identity`

Investigated candidate `b14f290` and [acceptance run 32728683722](https://github.com/YPQuinn/skills-management/actions/runs/32728683722).

- Reproduced both Chromium journey failures in local Google Chrome by delaying assertions 200ms: the transient `Checking…` button was already gone, and the recovered Skill's `unbound` text matched three elements after detail rendering. Replaced the transient-state assertion with the real Check response and retained final-state verification; restricted existing bounded retries to the observed Source-check lock contention. Recovery now checks the detail's explicit unbound explanation.
- Removed diagnostic delays/configuration after verification. Google Chrome: both journeys passed five consecutive repetitions (10/10). Full local Chromium/Firefox/WebKit suite: 8/8 passed. `./scripts/check.sh`: passed, including 144 frontend tests, fresh build, gofmt, vet, and all Go tests. `git diff --check`: passed.
- The macOS amd64 job waited for a `macos-13` runner for 24 hours without being assigned. Updated it to the documented x64 label `macos-15-intel`; updated arm64 functional CI to `macos-15`. The macOS 14 WebKit failure (`Unknown setting: PushAPIEnabled`) occurred at browser page creation; installed Playwright uses a legacy WebKit revision override on macOS 14. Local WebKit passes on macOS 26, but the replacement CI runner still needs remote verification.
- This is not final candidate acceptance: four-platform CI must run against the candidate containing these fixes, and remaining release gates/artifacts still need verification. Keep this ticket claimed.

### Candidate acceptance and PR follow-up

- [PR #1](https://github.com/YPQuinn/skills-management/pull/1) is open against `main`; no merge or release has been performed.
- Candidate `ac57e49908c30465421ecb61669e1b953a98afa5` passed [acceptance run 34078733265](https://github.com/YPQuinn/skills-management/actions/runs/34078733265): Linux amd64 quality/race/archive/Playwright, macOS arm64 quality/archive/Playwright, and native Linux arm64 plus macOS amd64 archive smokes. The replacement macOS runner and WebKit passed remotely.
- Built a binary explicitly reporting that candidate SHA and ran `scripts/smoke/readme.sh` with `SKILLCTL_BIN` pointing to it: passed in an isolated HOME.
- Agent-executed public GitHub smoke passed using that binary and a temporary HOME: registered `https://github.com/vercel-labs/agent-skills.git` with `--subpath skills/web-design-guidelines`, discovered and imported `web-design-guidelines`, verified Store `SKILL.md`, and checked again with `sync_status=in_sync` and `sync_stale=false`. Observed upstream commit: `063bee94c3f4df8453406c830b0a7df0f2860278`. The initial whole-repository registration exceeded the external 120-second command budget; the explicit-subpath run completed. This agent run does not replace the human-performed public smoke required by ticket 09.
- Corrected `docs/safety.md` to describe the recorded raw target plus physical symlink identity for ownership and adoption, matching this branch's implementation.
- Release blocker: both local and remote annotated `v0.1.0` resolve to `e0d6326a967324ef2c3ba2666916ba259c051531`, which predates these fixes. No GitHub Release exists. Existing local `dist/skillctl_v0.1.0_*` archives are not accepted as artifacts for the fixed candidate. Preserve the published tag unless the user explicitly decides otherwise; proposed next version is `v0.1.1`, pending confirmation and corresponding release-documentation updates.
- Remaining: validate the final candidate after documentation/version changes, obtain human public-smoke evidence, run exact-tag release packaging, and verify final versioned archives on native runners before publishing. Native CI smokes above build candidate binaries, not the final versioned release archives. Keep this ticket claimed.

### Approved release version

The user approved `v0.1.1` instead of moving the existing `v0.1.0` tag. Preserve `v0.1.0` at `e0d6326`; use a new exact `v0.1.1` tag on the final accepted release commit. README archive examples now use `v0.1.1`; the release script's usage describes its existing generic SemVer argument. This supersedes ticket 09's initial `v0.1.0` designation, not its acceptance requirements. Version approval does not close the outstanding human smoke or final native archive gates.

### Human acceptance and pre-merge review

The user confirmed successful manual public-GitHub registration/import/recheck using the binary reporting `db09150`. Both branch [run 34080086443](https://github.com/YPQuinn/skills-management/actions/runs/34080086443) and PR merge-ref [run 34080089318](https://github.com/YPQuinn/skills-management/actions/runs/34080089318) passed all four native platform jobs.

Pre-merge inspection then reproduced a remaining create-time ownership race: an external same-raw replacement before `CreateLink` samples the visible slug is incorrectly certified and can subsequently be deleted. See [ticket 21](21-fix-create-link-identity-race.md) for the deterministic failing reproduction. PR merge, tag creation, and release were paused despite successful existing CI and human smoke.

Ticket 21 is now resolved by `dff5265`: staged identity is durable before publication, and branch [run 34085004614](https://github.com/YPQuinn/skills-management/actions/runs/34085004614) plus PR merge-ref [run 34085006535](https://github.com/YPQuinn/skills-management/actions/runs/34085006535) passed all four native platform jobs. The prior create-time release blocker is closed. Keep ticket 19 claimed for final merged/tagged candidate, exact versioned archives, checksums, and publication.

### Merge and final-archive acceptance wiring

PR #1 was merged as `5cdb3a3d3de616c262f86213e225004780c3047a` after both branch and merge-ref four-platform checks passed for `00fa87b`. The existing native CI jobs rebuild temporary archives independently; those results alone cannot certify the exact files later uploaded as a release.

The `release archives` workflow closes that artifact gap: a tag-only invocation runs the existing exact-tag `release.sh` once, uploads its four archives and checksums, then four native runners download and checksum/execute those same files without rebuilding. It has read-only repository permissions and never publishes a Release. This workflow must be merged and successfully exercised for the final tag; static validation is not artifact acceptance. No `v0.1.1` tag or Release has yet been created.
