# Document and validate the MVP release candidate

Type: task
Status: claimed
Blocked by: 18

## Question

Finish the destination gate against one release candidate: write and verify the README, safety guide, troubleshooting guide, and Cobra help; run the complete check, native archive, CLI, REST, WebUI, recovery, concurrency, and Store-recovery gates; perform the public GitHub Source smoke; resolve release-blocking findings; and produce the final `v0.1.0` archives and checksums with recorded validation results.

## Comments

### CI follow-up on `fix/linux-ci-link-identity`

Investigated candidate `b14f290` and [acceptance run 32728683722](https://github.com/YPQuinn/skills-management/actions/runs/32728683722).

- Reproduced both Chromium journey failures in local Google Chrome by delaying assertions 200ms: the transient `Checking…` button was already gone, and the recovered Skill's `unbound` text matched three elements after detail rendering. Replaced the transient-state assertion with the real Check response and retained final-state verification; restricted existing bounded retries to the observed Source-check lock contention. Recovery now checks the detail's explicit unbound explanation.
- Removed diagnostic delays/configuration after verification. Google Chrome: both journeys passed five consecutive repetitions (10/10). Full local Chromium/Firefox/WebKit suite: 8/8 passed. `./scripts/check.sh`: passed, including 144 frontend tests, fresh build, gofmt, vet, and all Go tests. `git diff --check`: passed.
- The macOS amd64 job waited for a `macos-13` runner for 24 hours without being assigned. Updated it to the documented x64 label `macos-15-intel`; updated arm64 functional CI to `macos-15`. The macOS 14 WebKit failure (`Unknown setting: PushAPIEnabled`) occurred at browser page creation; installed Playwright uses a legacy WebKit revision override on macOS 14. Local WebKit passes on macOS 26, but the replacement CI runner still needs remote verification.
- This is not final candidate acceptance: four-platform CI must run against the candidate containing these fixes, and remaining release gates/artifacts still need verification. Keep this ticket claimed.
