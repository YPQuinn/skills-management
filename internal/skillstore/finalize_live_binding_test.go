package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestFinalizeRejectsForeignLiveSwapBeforeBaselineMoves proves the final
// live binding: a byte-identical foreign live tree swapped in after the
// first live proof and before the Baseline moves is refused by the second
// binding. The operation directory and its proof stay in place (the
// journal is preserved for conservative recovery), the Baseline installed
// by the proven phase is retained, and the foreign live tree is never
// touched.
func TestFinalizeRejectsForeignLiveSwapBeforeBaselineMoves(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	s.hook = func(p HookPoint) {
		if p != HookBeforeBaselineMoves {
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

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live swapped during finalization: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the byte-identical foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation journal (staging and proof) must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha")+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the Baseline installed by the proven phase must be retained: %q, %v", data, rerr)
	}
}

// TestFinalizeReplaceRejectsForeignLiveSwapBeforeBaselineMoves proves the
// final live binding across a replace: the Baseline advance and the
// previous-snapshot rotation of the proven phases are retained, the
// operation directory and its proof stay in place, and the foreign live
// tree is preserved.
func TestFinalizeReplaceRejectsForeignLiveSwapBeforeBaselineMoves(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	s.hook = func(p HookPoint) {
		if p != HookBeforeBaselineMoves {
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

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live swapped during replace finalization: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the operation journal (staging and proof) must be preserved")
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the byte-identical foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha")+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the Baseline advance must be retained: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the rotated previous snapshot must be retained: %q, %v", data, rerr)
	}
}

// TestFinalizeRejectsForeignLiveSwapAtRemoveOp proves the post-removal
// live binding closes the strict-removal window: a byte-identical foreign
// live tree swapped in at the strict-removal hook is refused after the
// operation children were removed but before the install proof is deleted.
// The operation directory and its proof stay in place (the journal is
// preserved for conservative recovery), the Baseline advance and
// previous-snapshot rotation of the proven phases are retained, and the
// foreign live tree is preserved.
func TestFinalizeRejectsForeignLiveSwapAtRemoveOp(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	s.hook = func(p HookPoint) {
		if p != HookBeforeRemoveOp {
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

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign live swapped at the strict removal: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the byte-identical foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha")+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the install proof must survive the refused final removal")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the Baseline advance must be retained: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the rotated previous snapshot must be retained: %q, %v", data, rerr)
	}
}

// TestFinalizeForeignChildAppearsBeforeStrictRemoval proves the strict
// operation removal is two-phase: a foreign child that appears right
// before the strict scan is refused with zero mutation, so the install
// proof and the foreign child are both preserved and the journal stays
// committed.
func TestFinalizeForeignChildAppearsBeforeStrictRemoval(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	s.hook = func(p HookPoint) {
		if p != HookBeforeRemoveOp {
			return
		}
		if err := os.WriteFile(filepath.Join(s.opDir(op.ID), "foreign.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign child before the strict removal: got %v, want ErrAmbiguous", err)
	}
	// the allowlist refusal happened before any known evidence was removed:
	// the install proof still exists
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the install proof must survive the refused strict removal")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the foreign child must be preserved: %q, %v", data, rerr)
	}
	// the proven phases completed before the cleanup refused
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("the Baseline must be installed before the cleanup refused: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the previous snapshot must be rotated before the cleanup refused: %q, %v", data, rerr)
	}
}
