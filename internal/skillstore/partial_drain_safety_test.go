package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// partialQuarantineState reproduces the crash state of a restore whose
// quarantine drain was interrupted: the installed live tree sits in the
// quarantine with part of its content already removed and a foreign child
// inserted after the crash. The drain cannot be resumed safely without a
// whole-tree manifest (the digest gate can no longer certify the
// remaining children), so the fresh process must refuse and preserve every
// candidate.
func partialQuarantineState(t *testing.T, s Store, op Operation, m materialized) {
	t.Helper()
	ctx := context.Background()
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer opDir.Close()
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	if err != nil {
		t.Fatal(err)
	}
	live.Close()
	if liveDigest != m.digest {
		t.Fatalf("test setup: live digest %s must equal the staged digest", liveDigest)
	}
	if _, err := movePinned(ctx, layout, layout.store, op.Slug, opDir, quarantineName, liveID, m.digest); err != nil {
		t.Fatal(err)
	}
	// the interrupted drain removed SKILL.md and a foreign child appeared
	// after the crash
	if err := os.Remove(filepath.Join(s.opDir(op.ID), quarantineName, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.opDir(op.ID), quarantineName, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreImportPartialQuarantineDrainIsAmbiguousPreservesAll proves a
// crash inside the quarantine drain leaves a state the fresh process
// cannot certify: the digest gate refuses (the remaining content no longer
// matches the proof), the injected foreign child and the remaining
// candidates are preserved, and the intent stays recoverable only through
// the same refusal.
func TestRestoreImportPartialQuarantineDrainIsAmbiguousPreservesAll(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	partialQuarantineState(t, s, op, m)

	_, err := New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial quarantine drain: got %v, want ErrAmbiguous", err)
	}
	q := filepath.Join(s.opDir(op.ID), quarantineName)
	if data, rerr := os.ReadFile(filepath.Join(q, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(q); err != nil {
		t.Fatal("the partially drained quarantine must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "baseline")); err != nil {
		t.Fatal("the Baseline candidate must be preserved")
	}
}

// TestRestoreImportPartialCandidateDrainIsAmbiguousPreservesAll proves the
// same refusal for the Baseline candidate: a candidate whose drain was
// interrupted no longer matches the proof digest, so the fresh process
// preserves it (with any injected foreign child) instead of resuming the
// drain by identity alone.
func TestRestoreImportPartialCandidateDrainIsAmbiguousPreservesAll(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	// the interrupted candidate drain removed SKILL.md and a foreign child
	// appeared after the crash
	if err := os.Remove(filepath.Join(s.opDir(op.ID), "baseline", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.opDir(op.ID), "baseline", "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial candidate drain: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), "baseline", "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// partialReplaceQuarantineState reproduces the crash state of a replace
// restore whose quarantine drain was interrupted: the installed new live
// tree sits in the quarantine with part of its content removed and a
// foreign child inserted after the crash.
func partialReplaceQuarantineState(t *testing.T, s Store, op Operation, m materialized) {
	t.Helper()
	ctx := context.Background()
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer opDir.Close()
	live, liveID, liveDigest, err := openTreeDigest(ctx, layout.store, op.Slug)
	if err != nil {
		t.Fatal(err)
	}
	live.Close()
	if liveDigest != m.digest {
		t.Fatalf("test setup: live digest %s must equal the installed digest", liveDigest)
	}
	if _, err := movePinned(ctx, layout, layout.store, op.Slug, opDir, quarantineName, liveID, m.digest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.opDir(op.ID), quarantineName, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.opDir(op.ID), quarantineName, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreReplacePartialQuarantineDrainIsAmbiguousPreservesAll proves
// the same fail-closed refusal for a pending replace: the injected foreign
// child, the partially drained quarantine, the recovery slot, and the
// operation evidence are all preserved.
func TestRestoreReplacePartialQuarantineDrainIsAmbiguousPreservesAll(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	partialReplaceQuarantineState(t, s, op, replacement)

	_, err := New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial replace quarantine drain: got %v, want ErrAmbiguous", err)
	}
	q := filepath.Join(s.opDir(op.ID), quarantineName)
	if data, rerr := os.ReadFile(filepath.Join(q, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(q); err != nil {
		t.Fatal("the partially drained quarantine must be preserved")
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation evidence must be preserved")
	}
}

// TestRestoreReplacePartialCandidateDrainIsAmbiguousPreservesAll proves
// the same refusal for a pending replace's Baseline candidate.
func TestRestoreReplacePartialCandidateDrainIsAmbiguousPreservesAll(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.Remove(filepath.Join(s.opDir(op.ID), "baseline", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.opDir(op.ID), "baseline", "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial replace candidate drain: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), "baseline", "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected foreign child must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
}
