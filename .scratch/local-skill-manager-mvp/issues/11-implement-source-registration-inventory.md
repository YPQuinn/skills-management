# Implement Source registration and inventory

Type: task
Status: resolved
Blocked by: 10

## Question

Implement the Local and Git Source vertical slice end to end: locator normalization, GitHub conveniences and ambient authentication, stable Git and Local observations, deterministic depth-three discovery, validation, whole-Source Inventory replacement, availability and staleness, Source registration/check/list/show, and the matching application, SQLite, CLI, REST, and Appica Source explorer behavior.

## Answer

Implemented the complete Local/Git Source slice. The core normalizes local paths, Git URLs, `owner/repo`, and `github:owner/repo`; rejects credential-bearing locators; uses runtime-only Git/SSH, token, and `gh` authentication without exposing tokens to plaintext HTTP; resolves branches, lightweight tags, annotated tags, default branches, and pinned commits; maintains locked partial-clone caches; and computes one canonical complete-tree content digest for Local and Git observations. Deterministic depth-three discovery validates frontmatter, records invalid candidates as Inventory issues, excludes traversal and special-node hazards, honors cancellation, and requires stable consecutive Local scans before accepting an observation.

SQLite migrations and application use cases persist Source identity, unique URL-safe names, observation metadata, availability/staleness, and coherent whole-Inventory replacements transactionally. The noun-first `skillctl source add|list|show|check` commands provide stable human and `--json` behavior, while strict versioned REST endpoints expose matching create/list/show/check operations through numeric resource IDs. The Appica Source explorer uses encoded name routes over the ID-based REST boundary, add-to-detail and check refresh flows, responsive master-detail tables with horizontal Scroll Areas, accessible loading/error/status states, and abort/stale-response guards for route and unmount races. Final independent review passed after closing Git credential transport, annotated-tag peeling, registration navigation-race, and file-size findings; frontend lint, 27 Vitest tests, production build, `go vet ./...`, normal and race Go suites, module tidiness, LOC limits, and whitespace checks all pass.
