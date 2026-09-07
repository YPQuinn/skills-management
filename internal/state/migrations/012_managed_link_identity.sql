-- Bind Managed Link ownership to the symlink's physical identity.
-- Raw target equality is not enough: a user-created replacement with the
-- same destination must stay unmanaged. Device and inode are not enough
-- either: ext4 can reuse an inode after unlink, so the recorded mtime
-- (nanoseconds) distinguishes a replacement even when the number matches.
-- Existing rows keep zeros (unproven) so the table is preserved; those
-- claims cannot prove ownership until the link is re-created or adopted.
ALTER TABLE managed_links ADD COLUMN link_dev INTEGER NOT NULL DEFAULT 0;
ALTER TABLE managed_links ADD COLUMN link_ino INTEGER NOT NULL DEFAULT 0;
ALTER TABLE managed_links ADD COLUMN link_mtime INTEGER NOT NULL DEFAULT 0;
