package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreImportSwapQuarantineAfterVerifyIsAmbiguousPreservesForeign is
// the critical quarantine TOCTOU regression: after the quarantine is opened
// and verified against the durable install proof, a byte-identical foreign
// tree is swapped into the quarantine slot before the drain. The re-proof
// detects the swap, refuses to drain the foreign object, and reports
// ErrAmbiguous with every candidate preserved.
func TestRestoreImportSwapQuarantineAfterVerifyIsAmbiguousPreservesForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

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
		t.Fatalf("post-verify quarantine swap: got %v, want ErrAmbiguous", err)
	}
	// the byte-identical foreign quarantine survives untouched
	q := filepath.Join(s.opDir(1), quarantineName)
	if data, rerr := os.ReadFile(filepath.Join(q, "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("byte-identical foreign quarantine must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("operation evidence must be preserved")
	}
}

// TestRestoreImportResumesQuarantineCleanup proves the crash state where
// the live tree was already moved into quarantine before its re-proof:
// recovery verifies the quarantine against the durable install proof,
// deletes it, and clears the operation artifacts.
func TestRestoreImportResumesQuarantineCleanup(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
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
		t.Fatalf("resume quarantine cleanup: %v", err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the quarantined install must be removed")
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("operation artifacts must be removed")
	}
}

// TestRestoreImportQuarantineRemovedIsSuccess proves the crash state where
// the quarantine was already deleted and only the operation artifacts
// remain: nothing of the pre-import state is left to restore, so recovery
// completes and the intent can be cleared.
func TestRestoreImportQuarantineRemovedIsSuccess(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("restore with the install already removed: %v", err)
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("operation artifacts must be removed")
	}
}
