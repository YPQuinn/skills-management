# Establish the runtime and bootstrap foundation

Type: task
Status: claimed
Blocked by: 09

## Question

Implement the runnable foundation selected by [Design the core architecture and REST boundary](08-design-core-architecture.md): Go composition root, domain and application boundaries, configuration, SQLite migrations, bootstrap/initialization, cross-process locks, Cobra/chi entry points, `/api/v1` setup boundary, and the root-mounted embedded Appica shell. Prove that `skillctl init`, uninitialized `skillctl ui`, initialization through `/setup`, restart, SPA deep links, API isolation, and loopback security work through both CLI and browser-facing seams.
