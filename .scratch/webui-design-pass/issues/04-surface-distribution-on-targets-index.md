# Surface Distribution health on the Targets index

Type: task
Status: resolved
Blocked by: 03

## Question

Show last Distribution outcome and observation staleness on the registered Targets table so the index is not only adapter/path metadata.

## Comments

- `state.Target` already stores `LastDistResult`, `LastInspectedStale`, and related timestamps; `ListTargets` currently drops them.
- Expose `last_result` and `stale` (omitempty) on the existing Target list JSON. Do not add a new resource or run Inspect on list.
- Replace or demote the Created column. Empty last_result reads as never distributed.
- Update REST/WebUI tests. CLI `--json` may gain omitempty fields; text list format stays the same.

## Answer

`ListTargets` now copies stored `LastDistResult` and `LastInspectedStale` onto the Target list JSON as `last_result` and `stale`. The Targets index shows that outcome (or “Never distributed”) instead of Created.
