package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestFinalizeContinuesAfterCandidateMoved proves the crash state where the
// candidate was already renamed into the Baseline path: the installed
// Baseline carries the proof's CandidateID, so finalization continues
// through the previous-snapshot rotation and completes.
func TestFinalizeContinuesAfterCandidateMoved(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	// crash state: the candidate was already moved into the Baseline path
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(opDir, "baseline", layout.baselines, "7"); err != nil {
		t.Fatal(err)
	}
	opDir.Close()
	layout.close()

	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatalf("finalize after the candidate was moved: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil {
		t.Fatal("the moved Baseline must be recognized as the candidate")
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("finalize must remove the operation directory")
	}
}

// TestFinalizeContinuesAfterPreviousRotated proves the crash state where
// the recovery slot was already rotated into the previous snapshot: the
// installed previous snapshot carries the proof's RecoveryID, so
// finalization continues and completes.
func TestFinalizeContinuesAfterPreviousRotated(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	// crash state: the recovery slot was already rotated into previous
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(layout.recovery, "2", layout.previous, "7"); err != nil {
		t.Fatal(err)
	}
	layout.close()

	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatalf("finalize after the rotation: %v", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the rotated previous snapshot: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.opDir(2)); !os.IsNotExist(err) {
		t.Fatal("finalize must remove the operation directory")
	}
}

// TestFinalizeBackupSwapBeforeDrainIsAmbiguous proves the moved-aside
// Baseline is drained only with the identity retained from this
// invocation's move: a byte-identical foreign baseline-old swapped in
// before the drain is refused and preserved with ErrAmbiguous.
func TestFinalizeBackupSwapBeforeDrainIsAmbiguous(t *testing.T) {
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
		backup := filepath.Join(s.opDir(2), "baseline-old")
		real := backup + ".real"
		if err := os.Rename(backup, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, backup); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign baseline-old before the drain: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(2), "baseline-old", "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign backup must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(2), "baseline-old.real", "SKILL.md")); err != nil {
		t.Fatal("the moved-aside Baseline must survive")
	}
}

// TestFinalizeStaleReappearsAfterDrainIsAmbiguous proves the stale previous
// snapshot is re-checked after its drain: a reappeared stale entry is
// foreign activity and is reported ErrAmbiguous with the entry preserved.
func TestFinalizeStaleReappearsAfterDrainIsAmbiguous(t *testing.T) {
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
		// the first transient drain belongs to the moved-aside Baseline;
		// the second belongs to the stale previous snapshot
		if p == HookAfterTransientDrain {
			n++
			if n == 2 {
				if err := os.MkdirAll(s.PreviousDir(7)+".old", 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	err := s.Finalize(context.Background(), op2)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("reappeared stale previous: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.PreviousDir(7) + ".old"); err != nil {
		t.Fatal("the reappeared stale entry must be preserved")
	}
}

// TestFinalizeUnknownOpChildBlocksCleanup proves the strict operation
// removal: a foreign child inside the operation directory after the moves
// completed is preserved with ErrAmbiguous instead of being drained by a
// fresh sample, and the journal stays committed. The allowlist refusal
// happens before any known evidence is removed, so the install proof still
// exists when the refusal is reported.
func TestFinalizeUnknownOpChildBlocksCleanup(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.WriteFile(filepath.Join(s.opDir(2), "foreign.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("unknown operation child: got %v, want ErrAmbiguous", err)
	}
	// the refusal happened before the known evidence was removed: the
	// install proof still exists
	if _, err := os.Lstat(filepath.Join(s.opDir(2), "proof")); err != nil {
		t.Fatal("the install proof must survive the refused strict removal")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(2), "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign child must be preserved: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the Baseline must be installed before the cleanup refused: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the previous snapshot must be rotated before the cleanup refused: %q, %v", data, rerr)
	}
}
