# Implement Skill import and Store materialization

Type: task
Status: claimed
Blocked by: 11

## Question

Implement explicit Skill import end to end: entry selection, slug validation and conflict handling, safe full-tree materialization, symlink and special-file rejection, size guards, Source Binding creation, per-Skill batch outcomes, atomic Store staging and replacement, Baseline/previous/recovery trees, and matching Skill views and import actions across CLI, REST, and Appica WebUI.

## Comments

- Stage B decision: accept fail-closed recovery for a crash during partial recursive quarantine/candidate drain. Recovery returns `ErrAmbiguous`, preserves filesystem evidence and the operation row, and requires manual intervention. Automatic convergence would require per-node ownership/progress equivalent to the explicitly excluded whole-tree manifest or event ledger; digest-only or root-ID-only deletion remains forbidden.
