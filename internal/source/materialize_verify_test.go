package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMaterializeLocalFinalVerifyRejectsDestinationSwap proves the
// destination-rooted final verification of a Local materialization is
// actually exercised: the deterministic race seam fires during the final
// TreeDigestRoot over the already-open destination handle, and a file
// swapped for an internal symlink at that window fails the import. The
// seam never fires for the absolute-path TreeDigest, so an implementation
// that fell back to re-opening the destination by path would not exercise
// the swap and this test would fail.
func TestMaterializeLocalFinalVerifyRejectsDestinationSwap(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "data.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, entry := observeLocalEntry(t, root, "skills/alpha")

	dst := t.TempDir()
	fired := false
	treeDigestRaceHook = func(p treeDigestHookPoint) {
		if p != hookRootFileBeforeOpen {
			return
		}
		if fired {
			return
		}
		fired = true
		file := filepath.Join(dst, "data.txt")
		if err := os.Rename(file, file+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("data.txt.real", file); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { treeDigestRaceHook = nil }()

	_, err := MaterializeEntry(context.Background(),
		Locator{Kind: KindLocal, Location: root}, "", entry, "", dst, false)
	if err == nil {
		t.Fatal("a destination file swapped at the final rooted verification: want error")
	}
	if !fired {
		t.Fatal("the final verification must run the rooted digest over the destination handle")
	}
	if _, err := os.Lstat(filepath.Join(dst, "data.txt.real")); err != nil {
		t.Fatal("the real materialized file must survive")
	}
	if info, err := os.Lstat(filepath.Join(dst, "data.txt")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the foreign symlink must be preserved, stat: %v, %v", info, err)
	}
}

// TestMaterializeGitFinalVerifyRejectsDestinationSwap proves the same
// destination-rooted final verification through the Git materialization
// path: the swap happens inside the final TreeDigestRoot over the opened
// destination, and the import must fail without accepting the foreign
// content.
func TestMaterializeGitFinalVerifyRejectsDestinationSwap(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/data.txt": {content: "payload", mode: 0o644},
	}
	bare, work := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()
	commit := strings.TrimSpace(gitRun(t, work, "rev-parse", "HEAD"))
	entry, _ := observeGitEntry(t, loc, workDir, "skills/alpha")

	dst := t.TempDir()
	fired := false
	treeDigestRaceHook = func(p treeDigestHookPoint) {
		if p != hookRootFileBeforeOpen {
			return
		}
		if fired {
			return
		}
		fired = true
		file := filepath.Join(dst, "data.txt")
		if err := os.Rename(file, file+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("data.txt.real", file); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { treeDigestRaceHook = nil }()

	_, err := MaterializeEntry(context.Background(), loc, commit, entry, workDir, dst, false)
	if err == nil {
		t.Fatal("a destination file swapped at the final rooted verification: want error")
	}
	if !fired {
		t.Fatal("the final verification must run the rooted digest over the destination handle")
	}
	if _, err := os.Lstat(filepath.Join(dst, "data.txt.real")); err != nil {
		t.Fatal("the real materialized file must survive")
	}
	if info, err := os.Lstat(filepath.Join(dst, "data.txt")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the foreign symlink must be preserved, stat: %v, %v", info, err)
	}
}
