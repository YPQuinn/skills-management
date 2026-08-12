package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreImportRejectsByteIdenticalForeignOpDirWithCopiedProof proves
// the restore proof closure: a byte-identical foreign operation directory
// carrying a copied install proof is refused by the physical identity of
// the operation directory itself (rootID(opDir) must equal the proof's
// opDirID) before any candidate or live tree is touched. The foreign
// directory, the live tree, and the original operation directory are all
// preserved.
func TestRestoreImportRejectsByteIdenticalForeignOpDirWithCopiedProof(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	opDir := s.opDir(op.ID)
	real := opDir + ".real"
	if err := os.Rename(opDir, real); err != nil {
		t.Fatal(err)
	}
	// a byte-identical foreign operation directory (proof included) takes
	// the staging slot
	swap := t.TempDir()
	if err := copyTreeForTest(swap, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, opDir); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign operation directory: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, "proof")); err != nil {
		t.Fatal("the foreign operation directory must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(opDir, "baseline", "SKILL.md")); err != nil {
		t.Fatal("the foreign Baseline candidate must be preserved")
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(real, "proof")); err != nil {
		t.Fatal("the original operation directory must survive beside the foreign one")
	}
}

// TestRestoreReplaceRejectsByteIdenticalForeignOpDirWithCopiedProof proves
// the same proof closure for a pending replace: the foreign operation
// directory, the live tree, and the recovery slot are all preserved.
func TestRestoreReplaceRejectsByteIdenticalForeignOpDirWithCopiedProof(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	opDir := s.opDir(op.ID)
	real := opDir + ".real"
	if err := os.Rename(opDir, real); err != nil {
		t.Fatal(err)
	}
	swap := t.TempDir()
	if err := copyTreeForTest(swap, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, opDir); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign operation directory: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, "proof")); err != nil {
		t.Fatal("the foreign operation directory must be preserved")
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the live tree must be preserved: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.recoveryDir(2), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the recovery slot must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(real, "proof")); err != nil {
		t.Fatal("the original operation directory must survive beside the foreign one")
	}
}

// TestRestoreReplaceNoLayoutLiveIsEmptyState proves a pending replace with
// no Store layout at all is the not-started state even when a live tree
// that still carries the old digest exists: Restore reports the empty
// receipt so the application can clear the intent through the abort
// protocol, and the live tree is never read, moved, or deleted here.
func TestRestoreReplaceNoLayoutLiveIsEmptyState(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: "new-digest"}
	// the live tree exists with the old digest but the Store layout never
	// existed: no operation evidence can attribute it, and none is needed
	// for the abort protocol
	live := liveDir(t, s, "alpha")
	if err := copyTreeForTest(live, old.dir); err != nil {
		t.Fatal(err)
	}

	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatalf("no-layout replace restore: %v", err)
	}
	if !receipt.Empty() {
		t.Fatalf("no-evidence replace must be the empty state: %+v", receipt)
	}
	if data, rerr := os.ReadFile(filepath.Join(live, "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the live tree must be preserved: %q, %v", data, rerr)
	}
}

// TestRestoreReplaceLayoutNoEvidenceIsEmptyState proves a pending replace
// whose layout exists but whose staging and recovery are absent is the
// not-started state even when the live tree still carries the old digest:
// the abort protocol clears the intent without ever claiming the live
// tree, which is preserved.
func TestRestoreReplaceLayoutNoEvidenceIsEmptyState(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	// a replace intent was persisted but never staged: no staging, no
	// recovery slot, and the live tree still holds the old content
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: "new-digest"}
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatalf("no-evidence replace restore: %v", err)
	}
	if !receipt.Empty() {
		t.Fatalf("no-evidence replace must be the empty state: %+v", receipt)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the live tree must be preserved: %q, %v", data, rerr)
	}
}
