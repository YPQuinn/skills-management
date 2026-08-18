-- Persist an unguessable private isolation directory (remove) and phase.
-- Create uses atomic symlinkat on the final slug and leaves slot_name
-- empty. A migrated v9 intent keeps slot_name='' and phase='planned'.
ALTER TABLE link_intents ADD COLUMN slot_name TEXT NOT NULL DEFAULT '';
ALTER TABLE link_intents ADD COLUMN phase TEXT NOT NULL DEFAULT 'planned';
