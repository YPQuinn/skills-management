package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCleanupTerminalRevalidatesResultAfterEvidenceRemoval proves the
// terminal result is re-validated after the receipt-bound evidence was
// removed: a byte-identical foreign live tree substituted at the final
// removal hook is refused with ErrAmbiguous and the foreign object is
// preserved, so the caller never clears the terminal row for a result it
// cannot prove.
func TestCleanupTerminalRevalidatesLiveAfterEvidenceRemoval(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		live := liveDir(t, s, "alpha")
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, live); err != nil {
			t.Fatal(err)
		}
	}

	err := cleanupFinalized(t, s, op, receipt)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live at the removal boundary: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign live tree must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha")+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
}

// TestCleanupTerminalRevalidatesBaselineAfterEvidenceRemoval proves the
// same post-removal re-validation for the installed Baseline of a replace:
// a byte-identical foreign Baseline substituted at the removal hook is
// refused and preserved.
func TestCleanupTerminalRevalidatesBaselineAfterEvidenceRemoval(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	op.SkillID = 7
	receipt, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		base := s.BaselineDir(7)
		real := base + ".real"
		if err := os.Rename(base, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, base); err != nil {
			t.Fatal(err)
		}
	}

	err = cleanupFinalized(t, s, op, receipt)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign Baseline at the removal boundary: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign Baseline must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7)+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed Baseline must survive beside the foreign one")
	}
}

// TestCleanupTerminalRevalidatesRestoredImportAfterEvidenceRemoval proves
// a restored import's terminal expectation also holds after the evidence
// removal: any live tree that appears at the removal hook is foreign and
// blocks the cleanup with the row preserved.
func TestCleanupTerminalRevalidatesRestoredImportAfterEvidenceRemoval(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedRestoreImport(t, s)
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		live := liveDir(t, s, "alpha")
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	op.Phase = PhaseRestored
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(context.Background(), op, parsed); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live after the import restore removal: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign live tree must be preserved: %q, %v", data, rerr)
	}
}

// TestCleanupTerminalRevalidatesPreviousAfterEvidenceRemoval proves the
// post-removal re-validation for the rotated previous snapshot of a
// replace: a byte-identical foreign previous substituted at the removal
// hook is refused and preserved.
func TestCleanupTerminalRevalidatesPreviousAfterEvidenceRemoval(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	op.SkillID = 7
	receipt, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		prev := s.PreviousDir(7)
		real := prev + ".real"
		if err := os.Rename(prev, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, prev); err != nil {
			t.Fatal(err)
		}
	}

	err = cleanupFinalized(t, s, op, receipt)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign previous at the removal boundary: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.PreviousDir(7), "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign previous snapshot must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.PreviousDir(7)+".real", "SKILL.md")); err != nil {
		t.Fatal("the rotated previous snapshot must survive beside the foreign one")
	}
}

// TestCleanupTerminalRevalidatesRestoredReplaceAfterEvidenceRemoval proves
// a restored replace's terminal expectation also holds after the evidence
// removal: a byte-identical foreign live tree substituted at the removal
// hook is refused and preserved.
func TestCleanupTerminalRevalidatesRestoredReplaceAfterEvidenceRemoval(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		live := liveDir(t, s, "alpha")
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, live); err != nil {
			t.Fatal(err)
		}
	}

	op.Phase = PhaseRestored
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(context.Background(), op, parsed); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live after the replace restore removal: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign live tree must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha")+".real", "SKILL.md")); err != nil {
		t.Fatal("the restored live tree must survive beside the foreign one")
	}
}

// TestCleanupTerminalValidatesTerminalResult proves the receipt-bound
// terminal expectations: for a finalized row the live tree and Baseline
// must still be the exact receipt-bound objects with the installed digest;
// a missing live tree, a byte-identical foreign live tree, or a corrupt
// digest is preserved with ErrAmbiguous and the row is kept.
