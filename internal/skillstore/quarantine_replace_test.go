package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreReplaceSwapQuarantineAfterVerifyIsAmbiguousPreservesAll is the
// critical replace quarantine TOCTOU regression: after the installed new
// tree is moved to quarantine and verified against the durable install
// proof, a byte-identical foreign tree is swapped into the quarantine slot
// before the drain. The re-proof detects the swap, refuses to drain the
// foreign object, and reports ErrAmbiguous with quarantine, recovery,
// candidate, and intent all preserved.
func TestRestoreReplaceSwapQuarantineAfterVerifyIsAmbiguousPreservesAll(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)

	s.hook = func(p HookPoint) {
		if p != HookAfterQuarantineOpen {
			return
		}
		q := filepath.Join(s.opDir(op.ID), quarantineName)
		swap := t.TempDir()
		if err := copyTreeForTest(swap, q); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(q); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, q); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("post-verify replace quarantine swap: got %v, want ErrAmbiguous", err)
	}
	// the byte-identical foreign quarantine survives untouched
	q := filepath.Join(s.opDir(2), quarantineName)
	if data, rerr := os.ReadFile(filepath.Join(q, "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("byte-identical foreign quarantine must be preserved: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.recoveryDir(2), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("recovery candidate must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.baselineCandidateDir(2), "SKILL.md")); err != nil {
		t.Fatal("Baseline candidate must be preserved")
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatal("operation evidence must be preserved")
	}
}

// TestRestoreReplaceResumesQuarantineCleanup proves the crash state where
// the installed new tree was already moved into quarantine: recovery
// verifies the quarantine against the durable install proof, deletes it,
// restores the old content from the proven recovery slot, and clears the
// operation artifacts.
func TestRestoreReplaceResumesQuarantineCleanup(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(layout.store, "alpha", opDir, quarantineName); err != nil {
		t.Fatal(err)
	}
	layout.close()

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("resume replace quarantine cleanup: %v", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("old content must be restored: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); !os.IsNotExist(err) {
		t.Fatal("the recovery slot must be consumed")
	}
	if _, err := os.Lstat(s.opDir(2)); !os.IsNotExist(err) {
		t.Fatal("operation artifacts must be removed")
	}
}

// TestRestoreReplaceQuarantineRemovedRestoresOld proves the crash state
// where the quarantine was already deleted and only the old recovery slot
// remains to be restored.
func TestRestoreReplaceQuarantineRemovedRestoresOld(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("restore with the install already removed: %v", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("old content must be restored: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); !os.IsNotExist(err) {
		t.Fatal("the recovery slot must be consumed")
	}
	if _, err := os.Lstat(s.opDir(2)); !os.IsNotExist(err) {
		t.Fatal("operation artifacts must be removed")
	}
}
