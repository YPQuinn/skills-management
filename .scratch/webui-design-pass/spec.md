# WebUI design pass

The WebUI is a localhost operator surface for Skill Manager. List pages currently lead with create forms, hide already-fetched Sync Status, and leave Skill Synchronization as raw hex plus three stacked diffs.

This pass:

1. Fixes the dark-mode tab contrast bug and removes the dead Skill Distribution tab.
2. Surfaces Sync Status on Skills and last Distribution outcome on Targets.
3. Moves Source/Group/Target creation into Appica Dialogs so registered data is first.
4. Drops redundant Name/Slug columns and uses relative timestamps.
5. Rewrites the Synchronization tab around conclusions, an explicit conflict Alert, path-aggregated diffs, and result language.
6. Applies unused Appica primitives (Skeleton, Copy Button, Tooltip, Toast, Combobox) where they replace current gaps.
