# Appica polish: Skeleton, Copy, Tooltip, Toast, Combobox, Delete, nav overflow

Type: task
Status: resolved
Blocked by: 08

## Question

Fill remaining operator-friction gaps with Appica primitives already in the design docs: loading skeletons, copy on digests/paths, term tooltips, success toasts, searchable assignment pickers, quieter Delete, and a visible overflow for main nav on narrow widths.

## Comments

- Skeleton instead of a lone Spinner on first list load.
- Copy Button on digest and path fields (after ticket 07 collapsible digests).
- TooltipProvider + tooltips for Baseline, Managed Link, Desired Skill Set.
- ToastProvider for successful Distribute / Sync / Import; keep destructive/conflict errors as in-page Alerts.
- Combobox for Target assignment Skill/Group pickers (searchable). Update e2e `chooseSelect` / keyboard assignment if the control role changes.
- Detail Delete: `outline` or `ghost` plus error color, not solid destructive as the highest-contrast control.
- Main nav: do not hide the scrollbar with no substitute. Show overflow or wrap so Groups/Targets remain discoverable under ~768px.

## Answer

List first-load uses Skeleton. Digest Collapsible rows have Copy. Baseline, Managed Link, and Desired Skill Set have term tooltips. Successful Import/Sync/Distribute fire toasts; errors stay Alerts. Target assignment Skill/Group pickers are searchable Comboboxes. Detail Delete is outline plus error color. Main nav keeps a visible horizontal overflow.
