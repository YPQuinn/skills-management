package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreRejectsSymlinkedStagingPreservesExternal proves a post-layout
// symlink swap of .skillctl/staging is refused before any mutation: the
// external directory a symlink points at survives untouched and the real
// staging evidence is preserved.
func TestRestoreRejectsSymlinkedStagingPreservesExternal(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.InternalDir(), "staging")
	real := staging + ".real"
	if err := os.Rename(staging, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, staging); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("symlinked staging: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "sentinel.txt")); err != nil {
		t.Fatal("external content must survive")
	}
	if _, err := os.Lstat(filepath.Join(real, "1", "tree")); err != nil {
		t.Fatal("real staging evidence must be preserved")
	}
}

// TestLayoutPinnedHandlesSurvivePathSwaps proves the pinned layout
// guarantee: once the layout handles are open, replacing internal
// directories with external symlinks cannot redirect any read, write, or
// deletion through those handles outside the Store.
func TestLayoutPinnedHandlesSurvivePathSwaps(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	// swap the recovery and baselines paths to external symlinks after the
	// handles were pinned
	for _, name := range []string{"recovery", "baselines"} {
		src := filepath.Join(s.InternalDir(), name)
		if err := os.Rename(src, src+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, src); err != nil {
			t.Fatal(err)
		}
	}

	// writes through the pinned handles land in the pinned directories
	r, _, err := ensurePinnedDir(layout.recovery, "77", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	r, _, err = ensurePinnedDir(layout.baselines, "88", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "recovery.real", "77")); err != nil {
		t.Fatal("pinned recovery write must land in the pinned directory")
	}
	if _, err := os.Lstat(filepath.Join(outside, "77")); !os.IsNotExist(err) {
		t.Fatal("writes must never land outside the Store")
	}
	if _, err := os.Lstat(filepath.Join(outside, "sentinel.txt")); err != nil {
		t.Fatal("external content must survive")
	}
	// deletion through a pinned handle stays contained
	if err := drainTree(layout.recovery, "77", fileID{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "77")); !os.IsNotExist(err) {
		t.Fatal("deletion must never reach outside the Store")
	}
}
