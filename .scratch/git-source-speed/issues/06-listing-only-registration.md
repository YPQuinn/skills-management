# Defer complete-tree digest until check or import

Type: task
Status: resolved
Blocked by: 05

## Question

Make Git `source add` persist Inventory names, descriptions, and relative directories by reading `SKILL.md` blobs only. Complete-tree blob download and digest stay on `source check` and `skill import`.

## Comments

- This is the only ticket that changes registration content.
- Import must refuse to reuse a listing-only Inventory (empty entry digests) and take a full observation before Store writes.
- Local Sources keep today’s full observation; they are not the slow path.

## Answer

Git `AddSource` calls `ObserveListing` (`discover mode=listing`). Entry digests are empty until `source check` or `skill import`. Local add is unchanged. After-opt add of 37 Skills: listing discover 16ms after clone. E2E `TestCLISourcesGitLifecycle` now asserts empty digests on add and filled digests on check.
