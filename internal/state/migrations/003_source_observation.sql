-- Source check observation metadata (decision 05): every check records its
-- start and completion (last_checked_at), result, error, observed Git
-- revision (last_commit), aggregate Inventory digest, and the latest
-- successful observation time (last_successful_check_at). Per-entry tree
-- digests live on the Inventory rows. A failed check updates only the
-- metadata below plus availability; it never touches the commit, digest, or
-- Inventory columns.
ALTER TABLE sources ADD COLUMN last_check_started_at TEXT;
ALTER TABLE sources ADD COLUMN last_check_result TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN last_inventory_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE source_inventory ADD COLUMN digest TEXT NOT NULL DEFAULT '';
