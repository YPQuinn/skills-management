# Invert list-page IA: data first, create in Dialog

Type: task
Status: resolved
Blocked by: 04

## Question

Put registered Sources, Groups, and Targets on the first screen. Move Add Source, Create Group, and Register Target (including adapter detection) into Appica Dialogs opened from a header button.

## Comments

- Create is infrequent; it currently occupies the first viewport on Sources, Groups, and Targets.
- Use Appica `Dialog` (`docs/design/llms-full.md` Dialog). One header primary button per index. Empty states still offer the same button.
- Adapter detection belongs inside the Register Target dialog, not above the registered table.
- Update unit tests and `e2e/webui` (smoke, chromium-journey, keyboard) to open the dialog before filling Location / Group Name / Target fields.

## Answer

Sources, Groups, and Targets indexes now lead with the registered table. Add Source, Create Group, and Register Target (including adapter detection) open from a header button into an Appica Dialog. Unit tests and `e2e/webui` open that dialog before filling the create fields.
