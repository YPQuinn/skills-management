# Issue tracker: Local Markdown

Issues and specs for this repository live as Markdown files in `.scratch/`.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`
- The spec is `.scratch/<feature-slug>/spec.md`
- Implementation issues are one file per ticket at `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, numbered from `01`
- Triage state is recorded as a `Status:` line near the top of each issue file
- Comments and conversation history append under a `## Comments` heading

## Publishing and fetching

- To publish an issue, create a file under `.scratch/<feature-slug>/`.
- To fetch a ticket, read the referenced path or numbered issue file.

## Wayfinding operations

- **Map**: `.scratch/<effort>/map.md`
- **Child ticket**: `.scratch/<effort>/issues/NN-<slug>.md`
- **Type**: a `Type:` line records `research`, `prototype`, `grilling`, or `task`
- **Status**: a `Status:` line records `open`, `claimed`, or `resolved`
- **Blocking**: a `Blocked by: NN, NN` line records dependencies; a ticket is unblocked when all listed tickets are resolved
- **Frontier**: open, unblocked, unclaimed tickets ordered by ticket number
- **Claim**: set `Status: claimed` before beginning work
- **Resolve**: append the resolution under `## Answer`, set `Status: resolved`, and add a gist with a link under the map's `## Decisions so far`
