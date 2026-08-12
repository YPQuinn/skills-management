package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeTreeAt writes the given path→content map below dir.
func writeTreeAt(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTreeDigestRootMatchesAbsoluteDigest proves the rooted no-follow
// digest is identical to the absolute-path digest for the same content, so
// materialization verification and Store recovery share one identity.
func TestTreeDigestRootMatchesAbsoluteDigest(t *testing.T) {
	dir := t.TempDir()
	writeTreeAt(t, dir, map[string]string{
		"SKILL.md":     "---\nname: A\ndescription: one\n---\n",
		"tools/run.sh": "#!/bin/sh\n",
		"empty/.keep":  "",
	})
	abs, err := TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	got, err := TreeDigestRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("rooted digest %s does not match absolute digest %s", got, abs)
	}
}

// TestTreeDigestRootNeverFollowsSymlinks proves symlinks are recorded as
// symlink nodes, never opened: the rooted digest is identical to the
// absolute digest even when a link points at a file outside the tree.
func TestTreeDigestRootNeverFollowsSymlinks(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTreeAt(t, dir, map[string]string{"SKILL.md": "content"})
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	abs, err := TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	got, err := TreeDigestRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("rooted digest %s does not match absolute digest %s", got, abs)
	}
}

// TestOpenNoFollowFileRejectsSymlink proves the no-follow open primitive
// behind the rooted digest refuses a regular-file entry swapped for an
// internal symlink between classification and open: the hash can never be
// redirected through the link.
func TestOpenNoFollowFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := openNoFollowFile(root, "link"); err == nil {
		t.Fatal("opening a symlink without following it: want error")
	}
}

// TestOpenPinnedDirRejectsSymlink proves the pinned-open primitive behind
// the rooted digest refuses a directory entry swapped for a symlink before
// it is opened, so recursion can never follow the link out of the tree.
func TestOpenPinnedDirRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, _, err := openPinnedDir(root, "sub"); err == nil {
		t.Fatal("pinning a symlinked directory: want error")
	}
}

// TestNameStillRefersDetectsDirectorySwap proves the identity primitive
// behind the rooted digest's post-recursion re-validation: after a
// directory is pinned, swapping the parent entry to a different directory
// is detected by physical identity, never by name or digest.
func TestNameStillRefersDetectsDirectorySwap(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	parent, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	child, id, err := openPinnedDir(parent, "sub")
	if err != nil {
		t.Fatal(err)
	}
	child.Close()
	// swap the pinned directory for a fresh byte-identical one
	swap := filepath.Join(dir, "sub.real")
	if err := os.Rename(filepath.Join(dir, "sub"), swap); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := nameStillRefers(parent, "sub", id); err == nil {
		t.Fatal("a swapped directory must fail the identity re-proof")
	}
	if _, err := os.Lstat(swap); err != nil {
		t.Fatal("the original directory must survive")
	}
}
