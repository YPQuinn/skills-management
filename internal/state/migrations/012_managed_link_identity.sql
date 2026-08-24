-- Bind Managed Link ownership to the symlink's physical identity.
-- Raw target equality is not enough: a user-created replacement with the
-- same destination is a different inode and must stay unmanaged.
-- Existing rows keep 0,0 (unproven) so the table is preserved; those
-- claims cannot prove ownership until the link is re-created or adopted.
ALTER TABLE managed_links ADD COLUMN link_dev INTEGER NOT NULL DEFAULT 0;
ALTER TABLE managed_links ADD COLUMN link_ino INTEGER NOT NULL DEFAULT 0;
