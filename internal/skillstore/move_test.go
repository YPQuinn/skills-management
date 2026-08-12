package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// setupMoveFailure returns a Store layout whose staging/mytree holds the
// materialized tree with its identity and digest sampled, then modifies the
// tree content in place (preserving its inode) so a move's destination
// proof fails deterministically while the source identity check passes. The
// caller must install s.hook before calling this helper, because the layout
// retains the hook at open time.
func setupMoveFailure(t *testing.T, s Store, m materialized) (*storeLayout, fileID, string) {
	t.Helper()
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensurePinnedDir(layout.staging, "mytree", 0o755); err != nil {
		t.Fatal(err)
	}
	treeDir := filepath.Join(s.InternalDir(), "staging", "mytree")
	if err := copyTreeForTest(treeDir, m.dir); err != nil {
		t.Fatal(err)
	}
	_, id, digest, err := openTreeDigest(context.Background(), layout.staging, "mytree")
	if err != nil {
		t.Fatal(err)
	}
	// Modify the content in place (same inode): the source identity still
	// matches, but the destination digest after the move will not.
	if err := os.WriteFile(filepath.Join(treeDir, "SKILL.md"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	return layout, id, digest
}

// TestMovePinnedDestinationSwapAfterRenameIsAmbiguousPreservesForeign is
// the destination-open window regression: a byte-identical foreign tree is
// swapped onto the destination after the forward rename completed but
// before the destination is opened. The first destination sample is never
// rollback authority: the re-open fails the identity compare, the foreign
// destination is preserved in place with ErrAmbiguous, and the source slot
// stays empty.
func TestMovePinnedDestinationSwapAfterRenameIsAmbiguousPreservesForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	s.hook = func(p HookPoint) {
		if p != HookAfterMoveRename {
			return
		}
		dst := filepath.Join(s.InternalDir(), "baselines", "moved")
		real := dst + ".real"
		if err := os.Rename(dst, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, m.dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, dst); err != nil {
			t.Fatal(err)
		}
	}
	layout, id, digest := setupMoveSource(t, s, m)
	defer layout.close()

	_, err := movePinned(context.Background(), layout, layout.staging, "mytree", layout.baselines, "moved", id, digest)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign destination after rename: got %v, want ErrAmbiguous", err)
	}
	dst := filepath.Join(s.InternalDir(), "baselines", "moved")
	if data, rerr := os.ReadFile(filepath.Join(dst, "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the byte-identical foreign destination must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "staging", "mytree")); !os.IsNotExist(err) {
		t.Fatal("the source slot must stay empty")
	}
	if _, err := os.Lstat(filepath.Join(dst, "..", "moved.real")); err != nil {
		t.Fatal("the real moved tree must survive beside the foreign destination")
	}
}

// setupMoveSource returns a Store layout whose staging/mytree holds the
// materialized tree with its identity and digest sampled from the same open
// handle, ready for a move whose destination proof only fails if something
// is swapped in. The caller must install s.hook before calling this helper,
// because the layout retains the hook at open time.
func setupMoveSource(t *testing.T, s Store, m materialized) (*storeLayout, fileID, string) {
	t.Helper()
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensurePinnedDir(layout.staging, "mytree", 0o755); err != nil {
		t.Fatal(err)
	}
	treeDir := filepath.Join(s.InternalDir(), "staging", "mytree")
	if err := copyTreeForTest(treeDir, m.dir); err != nil {
		t.Fatal(err)
	}
	_, id, digest, err := openTreeDigest(context.Background(), layout.staging, "mytree")
	if err != nil {
		t.Fatal(err)
	}
	return layout, id, digest
}

// TestMovePinnedRollbackPreservesForeignDestination proves the rollback
// pre-proof: after a forward rename and a digest mismatch, a byte-identical
// foreign tree swapped onto the destination before the rollback is never
// moved back or deleted — the operation reports ErrAmbiguous and preserves
// the foreign destination and the empty source slot.
func TestMovePinnedRollbackPreservesForeignDestination(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	original := t.TempDir()
	s.hook = func(p HookPoint) {
		if p != HookBeforeMoveRollback {
			return
		}
		dst := filepath.Join(s.InternalDir(), "baselines", "moved")
		swap := t.TempDir()
		if err := copyTreeForTest(swap, original); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dst); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, dst); err != nil {
			t.Fatal(err)
		}
	}
	layout, id, digest := setupMoveFailure(t, s, m)
	defer layout.close()
	// Capture the pre-modification content from the materialized tree so
	// the hook can swap in a byte-identical foreign destination.
	if err := copyTreeForTest(original, m.dir); err != nil {
		t.Fatal(err)
	}

	_, err := movePinned(context.Background(), layout, layout.staging, "mytree", layout.baselines, "moved", id, digest)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("rollback with foreign destination: got %v, want ErrAmbiguous", err)
	}
	dst := filepath.Join(s.InternalDir(), "baselines", "moved")
	if data, rerr := os.ReadFile(filepath.Join(dst, "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the byte-identical foreign destination must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "staging", "mytree")); !os.IsNotExist(err) {
		t.Fatal("nothing may be moved back over the foreign destination")
	}
}

// TestMovePinnedRollbackRestoresEmptySource proves the normal rollback:
// after a forward rename and a digest mismatch, an empty source slot with a
// destination that still carries the moved object is restored through the
// full re-proven move (identity, no-replace rename, parent syncs, and
// identity/digest/name verification of the restored tree) and the source
// content is back in place.
func TestMovePinnedRollbackRestoresEmptySource(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	s.hook = func(p HookPoint) {
		if p != HookBeforeMoveRollback {
			return
		}
		// Restore the original content in place (same inode), so the
		// rollback's re-proof of the moved tree succeeds.
		if err := os.WriteFile(filepath.Join(s.InternalDir(), "baselines", "moved", "SKILL.md"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	layout, id, digest := setupMoveFailure(t, s, m)
	defer layout.close()

	_, err := movePinned(context.Background(), layout, layout.staging, "mytree", layout.baselines, "moved", id, digest)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("rollback with empty source: got %v, want ErrAmbiguous", err)
	}
	src := filepath.Join(s.InternalDir(), "staging", "mytree")
	if data, rerr := os.ReadFile(filepath.Join(src, "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the moved tree must be restored to the source: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "baselines", "moved")); !os.IsNotExist(err) {
		t.Fatal("the destination must be empty after the rollback")
	}
}

// TestMovePinnedRollbackRefusesOccupiedSource proves a source slot that
// gained foreign content after the rename is never overwritten: the moved
// tree stays at the destination and both candidates are preserved with
// ErrAmbiguous.
func TestMovePinnedRollbackRefusesOccupiedSource(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	s.hook = func(p HookPoint) {
		if p != HookBeforeMoveRollback {
			return
		}
		src := filepath.Join(s.InternalDir(), "staging", "mytree")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "foreign.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	layout, id, digest := setupMoveFailure(t, s, m)
	defer layout.close()

	_, err := movePinned(context.Background(), layout, layout.staging, "mytree", layout.baselines, "moved", id, digest)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("rollback over an occupied source: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.InternalDir(), "staging", "mytree", "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("foreign source content must be preserved: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.InternalDir(), "baselines", "moved", "SKILL.md")); rerr != nil || string(data) != "modified" {
		t.Fatalf("the moved tree must stay at the destination: %q, %v", data, rerr)
	}
}
