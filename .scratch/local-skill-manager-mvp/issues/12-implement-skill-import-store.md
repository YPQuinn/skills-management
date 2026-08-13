# Implement Skill import and Store materialization

Type: task
Status: resolved
Blocked by: 11

## Question

Implement explicit Skill import end to end: entry selection, slug validation and conflict handling, safe full-tree materialization, symlink and special-file rejection, size guards, Source Binding creation, per-Skill batch outcomes, atomic Store staging and replacement, Baseline/previous/recovery trees, and matching Skill views and import actions across CLI, REST, and Appica WebUI.

## Comments

- Stage B decision: accept fail-closed recovery for a crash during partial recursive quarantine/candidate drain. Recovery returns `ErrAmbiguous`, preserves filesystem evidence and the operation row, and requires manual intervention. Automatic convergence would require per-node ownership/progress equivalent to the explicitly excluded whole-tree manifest or event ledger; digest-only or root-ID-only deletion remains forbidden.
- Replace previews the existing Skill in Ticket 12. The full affected Groups/Targets preview is deferred to Ticket 13, which introduces Groups, Targets, Assignments, and desired-set expansion and is blocked by this ticket; Ticket 12 must not invent speculative empty impact fields before those resources exist.

## Answer

Implemented the complete Skill import vertical slice across the application core, durable Store journal, CLI, REST, and Appica WebUI. Imports support explicit Inventory selection or all entries, slug overrides, conflicts and explicit replacement, size guards, per-item batch outcomes, Source Bindings, full-tree safe materialization, atomic Store/Baseline/previous/recovery transitions, and fresh-process terminal receipt recovery. CLI and REST expose matching stable JSON results; the WebUI provides Skills master-detail views and accessible Source Inventory import, replacement confirmation, conflict queues, cancellation guards, and exact status summaries.

Validation passed for full and race Go suites, vet, Linux compilation, Appica WebUI lint/tests/build, formatting, pure-LOC limits, and an empty staged index. The accepted partial recursive-drain fail-closed decision remains documented above.
