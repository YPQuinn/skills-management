# Remove the dead Skill Distribution tab

Type: task
Status: resolved
Blocked by: 01

## Question

Delete the permanently disabled Distribution tab on Skill detail. Distribution stays on the Target page, which already owns Inspect / Preview / Distribute.

## Comments

- The tab has no `TabsContent` and is `disabled`. It teaches nothing and invites dead clicks.
- Update `skills.test.tsx` (enabled Sync / disabled Distribution) and any locale keys used only by that tab.
- Do not build a Skill-side Distribution implementation in this ticket.

## Answer

Removed the disabled Distribution tab and unused `tabDistribution` locale keys. Distribution remains on the Target page. Skill detail keeps Overview and Synchronization.
