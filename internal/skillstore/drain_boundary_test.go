package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreImportQuarantineInPlaceMutationBeforeDrainIsAmbiguous proves
// the quarantine digest is re-verified at the final destructive boundary:
// content injected into the same quarantine directory (same root/inode)
// after the earlier verification but before the drain is refused with
// ErrAmbiguous and preserved, never deleted by identity alone.
func TestRestoreImportQuarantineInPlaceMutationBeforeDrainIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	s.hook = func(p HookPoint) {
		if p != HookAfterQuarantineOpen {
			return
		}
		if err := os.WriteFile(filepath.Join(s.opDir(op.ID), quarantineName, "injected.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("in-place quarantine mutation: got %v, want ErrAmbiguous", err)
	}
	q := filepath.Join(s.opDir(op.ID), quarantineName)
	if data, rerr := os.ReadFile(filepath.Join(q, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(q, "SKILL.md")); err != nil {
		t.Fatal("the original quarantine content must be preserved")
	}
}

// TestRestoreImportCandidateInPlaceMutationBeforeDrainIsAmbiguous proves
// the Baseline candidate is re-verified at its final destructive boundary:
// content injected into the same candidate directory after the earlier
// verification (the quarantine hook window) is refused and preserved.
func TestRestoreImportCandidateInPlaceMutationBeforeDrainIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	s.hook = func(p HookPoint) {
		if p != HookAfterQuarantineOpen {
			return
		}
		if err := os.WriteFile(filepath.Join(s.opDir(op.ID), "baseline", "injected.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("in-place candidate mutation: got %v, want ErrAmbiguous", err)
	}
	cand := filepath.Join(s.opDir(op.ID), "baseline")
	if data, rerr := os.ReadFile(filepath.Join(cand, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(cand, "SKILL.md")); err != nil {
		t.Fatal("the original candidate content must be preserved")
	}
}

// TestFinalizeBaselineOldInPlaceMutationBeforeDrainIsAmbiguous proves the
// moved-aside Baseline is re-verified at its final destructive boundary:
// content injected into the same baseline-old directory in the transient
// drain hook window is refused and preserved.
func TestFinalizeBaselineOldInPlaceMutationBeforeDrainIsAmbiguous(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	s.hook = func(p HookPoint) {
		if p != HookBeforeTransientDrain {
			return
		}
		if err := os.WriteFile(filepath.Join(s.opDir(2), "baseline-old", "injected.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("in-place baseline-old mutation: got %v, want ErrAmbiguous", err)
	}
	backup := filepath.Join(s.opDir(2), "baseline-old")
	if data, rerr := os.ReadFile(filepath.Join(backup, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(backup, "SKILL.md")); err != nil {
		t.Fatal("the moved-aside Baseline must be preserved")
	}
}

// TestFinalizeStalePreviousInPlaceMutationBeforeDrainIsAmbiguous proves
// the stale previous snapshot is re-verified at its final destructive
// boundary: content injected into the same stale directory in the second
// transient drain hook window is refused and preserved.
func TestFinalizeStalePreviousInPlaceMutationBeforeDrainIsAmbiguous(t *testing.T) {
	s := newStore(t)
	first := buildMaterialized(t, map[string]string{"SKILL.md": "first"})
	importAndFinalize(t, s, 1, "alpha", first, 7)

	second := buildMaterialized(t, map[string]string{"SKILL.md": "second"})
	op1 := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: first.digest, NewDigest: second.digest}
	stageAndInstall(t, s, op1, second)
	op1.SkillID = 7
	if err := s.Finalize(context.Background(), op1); err != nil {
		t.Fatal(err)
	}

	third := buildMaterialized(t, map[string]string{"SKILL.md": "third"})
	op2 := Operation{ID: 3, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: second.digest, NewDigest: third.digest}
	stageAndInstall(t, s, op2, third)
	n := 0
	s.hook = func(p HookPoint) {
		if p != HookBeforeTransientDrain {
			return
		}
		// the first transient drain belongs to the moved-aside Baseline;
		// the second belongs to the stale previous snapshot
		n++
		if n != 2 {
			return
		}
		if err := os.WriteFile(filepath.Join(s.PreviousDir(7)+".old", "injected.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op2)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("in-place stale mutation: got %v, want ErrAmbiguous", err)
	}
	stale := s.PreviousDir(7) + ".old"
	if data, rerr := os.ReadFile(filepath.Join(stale, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the injected content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(stale, "SKILL.md")); err != nil {
		t.Fatal("the stale snapshot must be preserved")
	}
}
