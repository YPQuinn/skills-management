# Implement Groups, Targets, and Assignments

Type: task
Status: resolved
Blocked by: 12

## Question

Implement the organization and Target setup vertical slice: Group membership, built-in Target Adapter resolution and detection, safe custom Target registration, fixed physical-path identity, direct Skill and Group Assignments, desired-set expansion with explanations, and matching Group and Target workflows across the application core, SQLite, CLI, REST, and Appica resource explorer.

## Answer

Delivered the complete organization and Target setup slice. SQLite, the application core, CLI, and REST now support overlapping Groups and membership, the eight accepted built-in Target Adapters with on-demand advisory detection, safe fixed-physical-path built-in and custom Target registration, idempotent direct Skill and Group Assignments, and deduplicated desired-set expansion that preserves every contributing reason. Skill replacement previews now expose affected Groups and Targets.

The Appica resource explorers provide bilingual, accessible Group and Target workflows with stable name-based routes, adapter detection and registration, membership and Assignment controls, desired-set explanations, and replacement-impact previews. Target registration remains read-only, Distribution and destructive cleanup remain reserved for their later tickets, and concurrent duplicate Assignment creation is idempotent.

Validation passed with `go test ./...`, focused race tests, `go vet ./...`, Go formatting checks, 111 Vitest tests, the production WebUI build, Oxlint, and a final deep review approval.
