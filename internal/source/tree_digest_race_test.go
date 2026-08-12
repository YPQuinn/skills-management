package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestTreeDigestRootRaceFileSwapToSymlinkFails exercises the narrow
// deterministic race seam between a regular file's classification and its
// no-follow open: a regular file swapped for an internal symlink at that
// window must fail the digest, never hash through the link. The seam is
// package-private, so an implementation that fell back to the
// absolute-path TreeDigest (which never fires the hook) would let the swap
// go unexercised and this test would fail.
func TestTreeDigestRootRaceFileSwapToSymlinkFails(t *testing.T) {
	dir := t.TempDir()
	writeTreeAt(t, dir, map[string]string{"SKILL.md": "content", "sub/x.txt": "x"})
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file := filepath.Join(dir, "SKILL.md")
	fired := false
	treeDigestRaceHook = func(p treeDigestHookPoint) {
		if p != hookRootFileBeforeOpen {
			return
		}
		if fired {
			return
		}
		fired = true
		if err := os.Rename(file, file+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("SKILL.md.real", file); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { treeDigestRaceHook = nil }()

	if _, err := TreeDigestRoot(context.Background(), root); err == nil {
		t.Fatal("a file swapped for an internal symlink between classification and open: want error")
	}
	if !fired {
		t.Fatal("the deterministic race seam never fired; the digest walk must expose it")
	}
	if _, err := os.Lstat(file + ".real"); err != nil {
		t.Fatal("the real file must survive the swap")
	}
	if info, err := os.Lstat(file); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the foreign symlink must be preserved, stat: %v, %v", info, err)
	}
}

// TestTreeDigestRootRaceDirSwapToForeignFails exercises the deterministic
// race seam between a directory's pinned open and its recursion: the name
// is swapped for a fresh foreign directory while the pinned child handle
// stays open, so the recursion hashes the pinned directory and the
// post-recursion re-proof must refuse the swap.
func TestTreeDigestRootRaceDirSwapToForeignFails(t *testing.T) {
	dir := t.TempDir()
	writeTreeAt(t, dir, map[string]string{"sub/inner.txt": "real", "other.txt": "other"})
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	sub := filepath.Join(dir, "sub")
	fired := false
	treeDigestRaceHook = func(p treeDigestHookPoint) {
		if p != hookRootDirBeforeRecurse {
			return
		}
		if fired {
			return
		}
		fired = true
		foreign := filepath.Join(dir, "foreign-dir")
		if err := os.MkdirAll(foreign, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(sub, sub+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(foreign, sub); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { treeDigestRaceHook = nil }()

	if _, err := TreeDigestRoot(context.Background(), root); err == nil {
		t.Fatal("a directory swapped for a foreign directory during recursion: want error")
	}
	if !fired {
		t.Fatal("the deterministic race seam never fired; the digest walk must expose it")
	}
	if _, err := os.Lstat(sub + ".real"); err != nil {
		t.Fatal("the real directory must survive the swap")
	}
	if info, err := os.Lstat(sub); err != nil || !info.IsDir() {
		t.Fatalf("the foreign directory must be preserved, stat: %v, %v", info, err)
	}
}
