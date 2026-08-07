# Design the core architecture and REST boundary

Type: grilling
Status: open
Blocked by: 02, 03, 04, 05, 06, 07

## Question

How should Go packages, filesystem ownership, SQLite state, synchronization and Distribution services, REST/JSON resources, chi serving, and the embedded Appica application be separated so CLI and WebUI share one reliable core—including alignment of the Vite asset base, client-router basename, chi mount and SPA fallback, REST prefix, theme policy, and any CSP?

## Context

- [Research Appica's embedded-WebUI constraints](02-research-appica-embedded-webui.md)
