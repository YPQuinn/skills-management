# Implement cross-resource cleanup lifecycles

Type: task
Status: resolved
Blocked by: 14, 15

## Question

Complete the destructive and relationship-changing lifecycles across the implemented vertical slices: Source deletion with explicit detach, Skill detach and rebind, Skill replacement and deletion, Target deletion, Assignment and Managed Link cleanup, previews and confirmations, ownership-loss handling, and equivalent safe behavior through the core, CLI, REST, and Appica WebUI.

## Answer

Delivered the remaining destructive and relationship-changing lifecycles across the Go core, SQLite state, CLI, REST, and bilingual Appica WebUI.

- Source deletion is blocked while Skills are bound; `--detach-skills` / `detach_skills` detaches those Skills and never deletes them.
- Skill detach removes the Binding and Baseline and marks the Skill unbound; Groups, Assignments, and distributed links stay.
- Skill rebind chooses an explicit Source Inventory entry. Identical content becomes `in_sync`; different content enters `conflict` without overwriting the Store.
- Skill replacement remains the existing import `--replace` path with impact preview.
- Referenced Skill deletion is blocked unless `--cleanup` / `cleanup` first removes related Assignments and verifiable Managed Links. Ownership-lost Target paths are warned about and left untouched. A cleanup failure retains the Store Skill.
- Target deletion previews and removes every verifiable Managed Link, then drops the registration while keeping the container. A failed safe removal retains the Target.
- Group deletion is blocked while assigned unless `--unassign` / `unassign` removes those Assignments. Managed Links stay until the next explicit Distribution.
- CLI prints a preview and requires `--yes` (or a TTY confirmation). REST exposes GET `.../deletion` previews plus DELETE/POST actions. WebUI uses Appica Alert Dialogs with the same explicit detach/cleanup/unassign semantics.

Validation passed: `go test ./...`, WebUI Vitest/Oxlint/production build, and `git diff --check`.
