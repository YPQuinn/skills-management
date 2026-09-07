-- Persist the symlink identity a create sampled at symlinkat time, and
-- the Managed Link identity a remove must re-verify. Existing open
-- intents keep 0,0,0 (unproven): create recovery must not claim them;
-- remove recovery still consults the Managed Link row.
ALTER TABLE link_intents ADD COLUMN link_dev INTEGER NOT NULL DEFAULT 0;
ALTER TABLE link_intents ADD COLUMN link_ino INTEGER NOT NULL DEFAULT 0;
ALTER TABLE link_intents ADD COLUMN link_mtime INTEGER NOT NULL DEFAULT 0;
