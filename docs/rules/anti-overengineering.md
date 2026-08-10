---
name: anti-overengineering
description: Two modes. (1) Prevents AI over-engineering on simple dev/research tasks — adding signatures/hash-locks on top of git, writing 200-line admission thresholds, demanding written authorization for public datasets, over-validating config keys, stuffing try/except/fallback into training scripts, generating test files for test files, unbounded ledger/bible growth, proliferating v1/v2/v3 file copies instead of using git branches. (2) Removes AI slop from existing code — narration comments, broad exception catching, single-impl interfaces, factory-for-one, GenericButton anti-pattern, _v2/enhanced_ naming, compat shims with zero consumers, can't-fail test assertions, oversized modules >250 LOC. Use this skill whenever writing experiment code, configs, training scripts, tests, ledgers, dataset-handling code, fiction, OR when reviewing/cleaning existing AI-generated code. Auto-activates on any task involving versioning, signatures, hashes, validation, error handling, reproducibility, datasets, file naming, OR when the user says "remove slop", "clean AI code", "deslop", "clean up AI-generated code". When in doubt, trigger it — over-engineering is harder to self-detect than under-engineering.
---

# Anti-Overengineering

This skill operates in two modes:
- **Preventive**: when you are about to write code/config/tests, stop yourself from over-engineering (Part I).
- **Curative**: when you are reviewing or cleaning existing AI-generated code, detect and remove slop safely (Part II).

Both share one principle: **every line of code should pay for itself.** Preventive mode = don't write lines that don't pay. Curative mode = find and remove lines that don't pay.

---

## Part I: Don't write it in the first place

### The core diagnosis

Every over-engineering symptom you are about to add is **re-implementing a capability that already exists** — in a worse, hand-rolled form.

- Git already content-addresses every file with a cryptographic hash. It detects any change to any file, on its own, for free.
- Open licenses already pre-authorize use. The license text is the permission.
- Reproducibility in research means *reporting* what you did (splits, seeds, error bars) — not cryptographically signing your scripts.
- Config handling is solved: required keys crash at boot, optional keys get a default.

Before adding *any* defensive mechanism, ask: **does the tool I'm already using do this natively?** If yes, you are about to write a worse duplicate. Stop.

The unifying principle is YAGNI — you aren't gonna need it. Every line of presumptive code you write costs twice: once to write it, and forever after to maintain it. Presumptive code is also usually wrong — you guessed at a future that turns out differently — but it sticks around because nobody dares delete "precautions." So you save time twice over by not writing it.

### Symptom catalog (preventive)

If you catch yourself doing one of these, stop.

#### 1. Signing code or experiment ledgers
Attaching a digital signature or checksum to a ledger file, an experiment record, or a script header "to prove integrity." Git already content-addresses every object with a SHA. It is impossible to change the contents of any file without git knowing. A hand-rolled signature on top of git is a worse re-implementation — and it forces a manual recompute cycle every time you edit anything. Use `git log`, `git blame`, `git tag` as the integrity ledger. If you need provenance, a one-line `## provenance:` comment pointing at the commit hash. Never write a signature script.

#### 2. Hash-locking configs
Freezing a config file by writing its hash next to it, so any edit requires recomputing the hash. Symptom #1 applied to config. Git already detects any change to any file. The hash-lock forces a recompute cycle on every config tweak — the slowest possible iteration loop for hyperparameter search. Treat configs as code. Commit them. `git diff` is your freeze-diff. For genuinely immutable configs, `git tag run-2026-08-01`. Zero hash math.

#### 3. Long admission-threshold / "gate" code
A 200-line preflight check, validator, or "experiment admission" module. The instinct is "always put checks in, always handle the most general case" — but that leaves no room to think. Defensive code is itself a source of bugs, because you're just as likely to find a defect in the defensive code as in the code it guards. Each imagined failure mode spawns three more, and the gate never stops growing. Validate at the actual system boundary (API entry, data load, model init) — once, loudly, fail-fast — and trust the result downstream. If a gate is genuinely needed, it fits in 5 lines. If it's 200 lines, you're anticipating failures that can't happen.

#### 4. Proliferating v1/v2/v3 file copies
`train_v1.py`, `train_v2.py`, `train_final.py`, `train_final_FINAL.py`. Git is the versioning system. Branches are free, tags are free, the reflog recovers anything for 90 days. A file suffix is a worse versioning mechanism in every dimension: no diff, no history, no recovery, no blame. One file, `train.py`. Use `git branch experiment/foo` for divergent variants. Use `git tag` for milestones.

#### 5. Demanding written authorization for public datasets
Refusing to use a dataset until you've obtained a signed letter. An open license *is* the authorization. The license is a one-way declaration attached to the dataset; users are automatically bound by its terms simply by using the data, with no paperwork required. For MIT / Apache 2.0 / CC-BY / CC0, permission is pre-granted. Facts themselves are often uncopyrightable, which is why CC0 is the research norm. Written authorization is only needed when there is no license (default all-rights-reserved) or a restrictive one (CC BY-NC, ND, bespoke data-use agreement). Check the license field. If permissive, cite and proceed. If unclear, *then* ask.

#### 6. Over-validating config keys
200 lines of schema validation, range checks, type coercion. "Fail fast" means **validate required keys once at startup and crash loudly if missing**. It does **not** mean writing a schema validator for every optional knob. Config-separation-from-code is about *where config lives*, not *how exhaustively you validate it*. Silent fallbacks are an anti-pattern; so is over-validation — both obscure intent. Required keys → assert at boot, crash if absent. Optional keys → a documented default. Hyperparameter "range sanity" belongs in the training loop's first-100-step logging, not in a preflight.

#### 7. Stuffing try/except/fallback into training scripts
Every line wrapped in `try/except`, broad `except Exception:`, silent fallbacks, "if X fails, try Y, then Z" chains. During development, you want errors *obnoxious*, not silent. Silent fallbacks turn a 5-minute debugging session into a 5-hour one, because the failure surfaces ten thousand steps later as a model that "mysteriously" underperforms. Each `except` you add is a place a real bug can hide. Let it crash. A training script that fails loudly on the first OOM, the first NaN, the first shape mismatch, is *correct* — that failure is the signal. Wrap exactly one thing in try/except: the top-level call, so you can log the traceback and exit non-zero. Nothing else.

#### 8. Writing test files for test files
`test_train.py`, then `test_test_train.py`, "to make sure the tests are correct." It is impossible to test absolutely everything without the tests becoming as complicated and error-prone as the code. A test is a *bet* that pays off when behavior is violated. A test that verifies another test's assertions is a bet on a bet — it has no behavioral seam of its own. Tests exist to give *change confidence*, not as deliverables. Bad tests are worse than no tests: they give false security and turn refactors into week-long debugging. Test the behavior, once, at the seam where it could actually break. The CI exit code is the meta-test.

#### 9. Unbounded "ledger" or "worldbuilding bible" growth
A `WORLD.md`, `CHARACTERS.md`, `TIMELINE.md`, `LEDGER.md` that grows by 500 words every chapter and is never re-read. This is YAGNI applied to writing. The ledger is a presumptive feature — you're writing it "in case I need it later." Usually, you don't need it, or what you actually need is different from what you foresaw. An unbounded ledger that nobody reads is pure maintenance drag and token cost: it grows forever because it has no purpose that would constrain it. Write the story. When you hit a consistency question, grep the manuscript. If a fact recurs enough to need a canonical home, *then* promote it to a single `notes.md` entry — and prune anything that hasn't been referenced in the last 5 chapters.

---

## Part II: Remove slop from existing code

### What counts as slop

**The test:** would a strong senior engineer, writing this file from scratch in this codebase, have produced this line? If no → slop.

**The anti-over-correction test:** does removing it change behavior or hide intent? If yes → keep it.

Apply categories in this order (safest → riskiest): comments → dead code → defensive → duplication → complexity → abstraction/boundary → performance → tests → oversized-modules.

#### Comments & documentation
- Narration comments restating the next line (`// increment counter` above `counter++`), section dividers (`// ===== HELPERS =====`), commented-out code, file headers describing the filename → delete.
- Trivial docstrings repeating the signature (`/** Get the user */` above `getUser()`) → delete or shrink to one line.
- JSDoc/docstring > 3× the function body, or comment/code ratio > 2:1 → shrink.
- Vague TODOs (`// TODO: consider...`, `// TODO: implement` above already-implemented code) → delete.
- Debug leftovers (`console.log`, `print(...)`, `dbg!`, function-entry/exit logging) → delete.
- KEEP: comments explaining WHY (business logic, edge cases, workarounds, ticket links, regex/algorithm explanations), BDD markers (`# given`/`# when`/`# then`).

#### Defensive code
- Null checks for guaranteed values, isinstance on statically-typed params, default values for required params, redundant validation duplicated at multiple layers → delete inside the trust boundary.
- Triple null/undefined checks (`x !== null && x !== undefined && x !== ''`) → collapse to `if x:`.
- `except Exception` / empty `catch {}` / `catch (e) { log(e) }` without narrowing → catch the specific exception, or add instanceof narrowing + re-throw unknown.
- KEEP: validation at system boundaries (user input, HTTP, files, env, external APIs), I/O error handling, nullable DB fields, top-level boundary catch-all (CLI `main()`, HTTP handler) with logging + re-raise.
- ⚠️ Removing a reachable error-swallow IS a behavior change (inputs that used to get the default now raise) — flag it, don't apply it. Only delete a catch when the wrapped code provably cannot raise. Before deleting any guard at a trust boundary, require an adversarial regression (malformed/hostile input) that fails if the guard is removed. No adversarial test → the guard stays. A guard with no proof of redundancy is load-bearing.

#### Complexity
- Deep nesting (>3 levels) → guard clauses / early returns.
- God functions (>50 lines doing many things, or cyclomatic complexity >10) → split by responsibility.
- Long parameter lists (>5 args without a struct/dataclass) → group into a struct.
- Complex boolean (4+ predicates combined) → name the sub-conditions or split.
- `if/elif/else` chains for type/enum/literal discrimination → `match/case` + `assert_never` on the wildcard (Python), or equivalent exhaustive pattern matching.
- `object` used as a type annotation → `Protocol` (structural), `TypeVar` (generic), or explicit union.
- KEEP: established complexity patterns in the codebase, performance-critical hot paths. `if/else` for boolean conditions and range checks is fine (not variant discrimination).

#### Needless abstraction
- Pass-through wrappers, single-use helpers (called exactly once) → inline.
- Single-impl interfaces (`interface X` with one `XImpl` and no second implementation planned) → delete the interface, use the concrete type.
- Factory-for-one (`ConnectionFactory.createConnection()` with no variation) → direct construction.
- Builder for a 2-3 field object → direct construction.
- Strategy pattern with one strategy → call the function directly.
- Event/pub-sub systems where every event has exactly one subscriber → direct function call.
- "Manager"/"Service"/"Handler" class wrapping two functions → delete the class, use the functions.
- Options parameter with one call site that never passes it → delete the options.
- KEEP: abstractions with 2+ real current users; abstractions providing a testability seam; framework-required boundaries; patterns the codebase already uses. Test: if removing the abstraction makes the code shorter AND equally readable, it was unnecessary.

#### Dead code & duplication
- Unused imports, unused private functions/methods, unreachable branches, stale feature flags, `else` after exhaustive returns → delete.
- Copy-pasted branches with trivial differences → deduplicate.
- Reimplemented util that already exists in the repo (date formatting, slugify, debounce, retry, deep-merge, validation) → search the repo first (`rg -i 'formatDate|format_date'`), use the existing one even if you'd write it differently.
- KEEP: code referenced via reflection/dynamic dispatch/string lookup; feature-flag rollback paths (verify with user). Incidental duplication (two pieces that look similar but serve different intents that could diverge) — prefer leaving them separate over forcing a premature shared abstraction.

#### Naming
- Revision prefixes/suffixes (`enhanced_`, `improved_`, `new_`, `_v2`, `final_`) → name for what it is, not its revision history.
- Vague verbs (`processData`, `handleItem`, `doWork`, `manageState`) → name the actual transformation (`parseInvoiceRows`, `dedupeByEmail`).
- Generic placeholders in production code (`data`, `result`, `info`, `temp`, `helper`) → rename to what they hold. (Generic names in tiny scopes like `i`, `x`, `acc` in a 3-line function are fine.)
- Type-in-name (`userObject`, `resultArray`, `stringData`) → shorten to what's unambiguous in scope.
- Suffix abuse (`*Helper`, `*Manager`, `*Util`, `*Wrapper` without justification) → rename to the concept, or delete the class if it's a wrapper.
- ⚠️ Rename safety: rename only symbols introduced by the slop code. Renaming public/exported symbols is an API change — flag, don't do.

#### Compat slop
- Backwards-compat shims / re-export aliases "for backwards compatibility" on brand-new code, deprecated wrappers with zero consumers, version checks for versions the project doesn't support → verify with a repo-wide search that nothing consumes them, then delete. If something DOES consume them, they're not slop — leave them.

#### Test slop
- Assertions that can't fail (`assert result is not None` right after constructing it, `expect(true).toBe(true)`, asserting a mock returned what you mocked it to return, tautological `expect(x).toBe(x)`) → rewrite to a real behavioral assertion or delete.
- Over-mocking (every collaborator mocked so the test exercises nothing real) → mock only the boundary, let real collaborators run.
- Implementation-detail tests (asserting a private method was called N times instead of asserting observable output) → rewrite to behavior assertions.
- "Doesn't throw" tests / snapshot tests standing in for behavioral assertions → replace with real assertions.
- ⚠️ Hard rule: never delete a failing or inconvenient test to make a suite green. That is the worst slop of all.

#### Performance equivalences (behavior-preserving)
- O(n²)→O(n) via set lookup vs list scan; repeated computation inside a loop → hoist outside; eager `list(...)` when only iterated once → generator; string concat in loop → `join`; redundant DB/API calls in a loop → batch; redundant deep copies; `.length`/`len()` recomputed inside loop → cache.
- Hard rule: only apply when behavior equivalence is obvious. Do NOT change algorithms with subtle correctness implications. Do NOT micro-optimize hot paths without a benchmark. If in doubt, SKIP.

#### Oversized modules
- Any source file exceeding **250 pure LOC** (non-blank, non-comment lines). Measure: `awk '!/^[[:space:]]*$/ && !/^[[:space:]]*(#|\/\/)/' <file> | wc -l`.
- This is an architectural defect, not a style preference. Identify distinct responsibilities (SRP), split by what each file DOES, name each file after the concept it owns (never `utils.py`/`helpers.py`/`common.py`/`part_1.py`), re-export via `__init__.py` (re-exports ONLY, no logic).
- Forbidden escapes: counting blanks/comments toward budget; splitting by token count; catch-all dump files; "it's generated" (only valid in build output dirs); "230 LOC, close enough" (a 230-LOC file about to grow is already over).
- KEEP: genuinely self-contained single-responsibility scripts. Opt out with `# noqa: SIZE_OK` in first 5 lines + explanation.

#### Missing tests
- Behavior present in changed files not locked by any regression test → ADD the narrowest test that pins the behavior. Do NOT remove code.
- EXCEPTION: a PROSE file (prompt, `SKILL.md`, rule, markdown) has no behavioral seam — do NOT add a text/word-count/phrase pin for it; that guards a diff, not behavior. Cover only a machine-consumed value, or leave it to review.

### Slop removal discipline

#### Lock behavior first (non-negotiable)
Before removing a single line, lock behavior with green tests. For each in-scope file: identify public/observable behavior (exported functions, handlers, CLI commands). If uncovered or weakly covered, write the narrowest regression test pinning current behavior BEFORE editing. Tests must be green before any cleanup begins. If you can't establish a green baseline (test runner broken), STOP and report. **A checklist alone is not safety; a passing regression test is.**

#### Run the deletion ladder before smell analysis
Before categorizing smells, run this on each changed unit (biggest, safest deletion first):
1. **Delete entirely** — the behavior is not needed (YAGNI, speculative, dead on arrival).
2. **Reuse** — an existing helper/pattern in the repo already does it.
3. **Platform/stdlib/native/dependency** — the stdlib/runtime/installed dep already does it (hand-rolled date picker → `<input type="date">`, custom query parser → `URLSearchParams`, bespoke debounce → the util already imported).
4. **Simplify in place** — it must exist; make it smaller.

Only code landing on "Simplify in place" proceeds to the smell categories above. One function replaced by a platform call is a bigger, safer win than any in-place cleanup — and it needs no per-line smell analysis.

#### Certainty levels
- **HIGH** (auto-fix): regex-detectable — debug statements, empty catch, placeholder text, trailing whitespace. Safe to remove directly.
- **MEDIUM** (flag for review): structural — doc-code ratio problems, stub functions, dead code, unused infrastructure. Needs context.
- **LOW** (report only): heuristic — over-engineering, buzzword inflation. Context-dependent.

After auto-fixing HIGH items, run the test suite; if tests fail, roll back with `git restore .`.

#### What NOT to remove (guardrails)
- Boundary validation/error handling — before deleting any guard at a trust boundary, require an adversarial regression. No adversarial test → the guard stays.
- Comments explaining WHY.
- Public API signatures.
- Type hints.
- Code referenced via reflection/dynamic dispatch/string lookup.
- Reachable error-swallows (removing them changes behavior — flag, don't apply).
- Code outside scope (report it, don't touch it).
- New abstractions/dependencies (never introduce during cleanup).
- Chesterton's fence — before deleting weird-looking code, `git blame` it. If you can't explain why it exists, you don't yet have permission to remove it. (Exception: code you wrote in the same session — you know its provenance.)

#### If cleanup breaks something
1. Identify the specific change that caused the failure.
2. Revert the affected file (or the problematic hunk via targeted edit).
3. Re-apply only the changes you can prove are safe.
4. Re-run the failing gate.
5. If you fail 3× on the same file, STOP and escalate to the user. Do not keep editing.

---

## The five pre-flight questions

Before adding any new file, any new layer of validation, any signature, any version suffix, any try/except — ask:

1. **Does git already do this?** (versioning, integrity, history, recovery)
2. **Does the license already grant this?** (use, modification, redistribution)
3. **Does the framework already provide this?** (logging, validation, config loading)
4. **Will this code run more than once?** (one-shot scripts don't need defensive layers)
5. **If I delete this, what breaks?** (if nothing, delete it)

If the answer to 1-3 is yes, you're about to duplicate a canonical tool. If the answer to 4 is no, you're building infrastructure for a one-shot. If the answer to 5 is "nothing," the code is dead weight.

---

## Pre-delivery checklist

Before declaring a task complete, verify:

- [ ] I changed only what was asked. No unrequested abstractions, no unrequested files.
- [ ] Every defensive mechanism I added maps to a real, named failure mode I have seen — not one I imagined.
- [ ] No file has a `_v2`, `_final`, `_NEW` suffix. Variants live in branches.
- [ ] No signature, hash-lock, or integrity layer that git doesn't already provide.
- [ ] Error handling lets failures surface loudly. No silent fallbacks.
- [ ] Tests cover behavior at real seams, not other tests.
- [ ] Config validation is "required keys crash at boot, optional keys get a default" — nothing more.
- [ ] Any "ledger" or "bible" file is pruned to only what's referenced.
- [ ] (If cleaning slop) Behavior was locked with green tests before any removal.
- [ ] (If cleaning slop) No public API signatures changed, no type hints removed.
- [ ] (If cleaning slop) Each removal maps to a named slop category above — not a vibe.

---

## Guardrails — do NOT over-correct

This skill is a brake, not a steering wheel. Do not use it to delete:

- **Necessary complexity.** A real distributed-systems protocol, a real cryptographic boundary, a real safety-critical path *does* need defensive code. The test is: is the complexity tied to a named, real requirement, or to an imagined one?
- **Domain complexity.** Some domains (ML training, numerical code, parsing) are just hard. Don't flatten them into "simpler" versions that are wrong.
- **Team conventions.** If the repo has an established pattern (e.g., always validates schemas at service boundaries), follow it — don't rip it out citing this skill.
- **Chesterton's fence.** Before deleting weird-looking code, `git blame` it. If you can't explain why it exists, you don't yet have permission to remove it. (Exception: code you wrote in the same session — you know its provenance.)

The goal is not minimal code at any cost. The goal is **code that pays for itself** — every line earns its place by mapping to a real requirement or a real failure mode.

---

## When this skill does NOT apply

- The user explicitly asks for a defensive, hardened, or production-grade system. Deliver what they asked for.
- The task is genuinely safety-critical (medical, aerospace, financial integrity). Use domain-appropriate standards.
- The user is paying for a formal audit or compliance artifact. Produce the artifact.
- The user explicitly asks you to keep defensive layers or not refactor. Respect that.

When in doubt, ask: "This looks like it could be simpler — want me to strip the defensive layers, or is the hardening intentional?"