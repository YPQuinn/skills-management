package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestStageMkdirOpenSwapForeignOpDirIsAmbiguousPreserved is the Mkdir→Open
// window regression: a foreign non-empty operation directory is planted
// after the exclusive Mkdir, before the created directory is opened. The
// foreign directory is never blessed as the operation's own staging: its
// pre-existing child makes the tree creation fail and the manifest cleanup
// must refuse to drain the foreign directory (unknown child), reporting
// ErrAmbiguous with the foreign content preserved.
func TestStageMkdirOpenSwapForeignOpDirIsAmbiguousPreserved(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	defer func() { testAfterMkdirHook = nil }()
	testAfterMkdirHook = func(parent *os.Root, name string) {
		if name != "1" {
			return
		}
		opDir := filepath.Join(s.InternalDir(), "staging", "1")
		if err := os.Rename(opDir, opDir+".real"); err != nil {
			t.Fatal(err)
		}
		// a foreign directory that already carries a "tree" child occupies
		// the slot before the open
		if err := os.MkdirAll(filepath.Join(opDir, "tree"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(opDir, "tree", "foreign.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign opDir in the Mkdir→open window: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.InternalDir(), "staging", "1", "tree", "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign operation directory must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "staging", "1.real")); err != nil {
		t.Fatal("the created operation directory must survive")
	}
}

// TestStageMkdirOpenSwapSymlinkRefused proves a symlink planted in the
// Mkdir→open window is refused before anything is pinned, the operation
// reports ErrAmbiguous, and neither the symlink target nor the created
// directory is touched.
func TestStageMkdirOpenSwapSymlinkRefused(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testAfterMkdirHook = nil }()
	testAfterMkdirHook = func(parent *os.Root, name string) {
		if name != "1" {
			return
		}
		opDir := filepath.Join(s.InternalDir(), "staging", "1")
		if err := os.Rename(opDir, opDir+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, opDir); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("symlink in the Mkdir→open window: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "sentinel.txt")); err != nil {
		t.Fatal("the symlink target must survive")
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "staging", "1.real")); err != nil {
		t.Fatal("the created operation directory must survive")
	}
}

// TestStageSourceOpenFailureCleansContract proves the ordinary-failure
// contract when the materialized source cannot be opened: the exact
// retained-identity cleanup runs, the operation directory is finally
// absent, and the failure stays ordinary (not ambiguous).
func TestStageSourceOpenFailureCleansContract(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})

	_, err := s.Stage(context.Background(), 1, filepath.Join(m.dir, "missing"), m.digest, false)
	if err == nil || errors.Is(err, ErrAmbiguous) {
		t.Fatalf("missing source must be an ordinary failure, got %v", err)
	}
	if _, statErr := os.Lstat(s.opDir(1)); !os.IsNotExist(statErr) {
		t.Fatalf("the operation directory must be absent after the proven cleanup, stat: %v", statErr)
	}
}

// TestStageCleanupOpDirReappearsIsAmbiguous proves the cleanup contract's
// final absence re-check: when the operation directory reappears after the
// exact cleanup drained it, the ordinary failure cannot be certified and
// the operation is reported ErrAmbiguous with the reappeared directory
// preserved.
func TestStageCleanupOpDirReappearsIsAmbiguous(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		if err := os.MkdirAll(s.opDir(1), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, dir, "digest", false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("reappeared operation directory after cleanup: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("the reappeared operation directory must be preserved")
	}
}

// TestStageCleanupRefusesUnknownChild proves the manifest cleanup never
// removes a node this invocation did not create: a foreign regular file
// swapped onto a staged file name during the copy fails the copy, and the
// cleanup must refuse the swapped node (identity mismatch) with
// ErrAmbiguous, preserving the foreign file and the created file.
func TestStageCleanupRefusesUnknownChild(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { testBeforeChmodFileHook = nil }()
	testBeforeChmodFileHook = func(parent *os.Root, name string) {
		if name != "SKILL.md" {
			return
		}
		tree := s.stagedTreeDir(1)
		if err := os.Rename(filepath.Join(tree, "SKILL.md"), filepath.Join(tree, "SKILL.md.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(foreign, "SKILL.md"), filepath.Join(tree, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign child blocking the cleanup: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.stagedTreeDir(1), "SKILL.md")); rerr != nil || string(data) != "foreign" {
		t.Fatalf("the foreign file must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.stagedTreeDir(1), "SKILL.md.real")); err != nil {
		t.Fatal("the created file must survive")
	}
}

// TestDiscardStagingMissingOpDirIsAmbiguous proves the opaque discard
// refuses an expected operation directory that vanished: nothing may be
// certified from an absent name, so the discard is ErrAmbiguous and the
// layout survives.
func TestDiscardStagingMissingOpDirIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(s.opDir(1)); err != nil {
		t.Fatal(err)
	}

	err = s.DiscardStaging(op.ID, staged)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("discard with a missing expected opDir: got %v, want ErrAmbiguous", err)
	}
	if err := s.EnsureLayout(); err != nil {
		t.Fatal("the layout must survive an ambiguous discard")
	}
}
