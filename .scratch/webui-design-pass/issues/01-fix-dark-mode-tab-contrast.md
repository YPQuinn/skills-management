# Fix dark-mode Skill tab contrast

Type: task
Status: resolved
Blocked by:

## Question

Remove the hardcoded `!text-black` on Skill detail `TabsTrigger`s so Overview / Synchronization remain readable in dark theme.

## Comments

- `internal/webui/src/skill-detail.tsx` forces `className="!text-black"` on all three tabs.
- Appica Tabs already use semantic foreground tokens. Delete the override; do not add a replacement color class.
- Add or extend a test that the tabs stay role=tab without a forced black text class.

## Answer

Removed `!text-black` from Skill detail tabs so they use Appica foreground tokens in light and dark theme. `skills.test.tsx` asserts the class is gone.
