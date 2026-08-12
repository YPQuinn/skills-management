package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizeImportInstallsBaseline(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{
		"SKILL.md":     "---\nname: Alpha\ndescription: one\n---\n",
		"tools/run.sh": "#!/bin/sh\n",
	})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7

	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	baseline := s.BaselineDir(7)
	if _, err := os.Lstat(filepath.Join(baseline, "SKILL.md")); err != nil {
		t.Fatalf("baseline missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(baseline, "tools", "run.sh")); err != nil {
		t.Fatalf("baseline tree incomplete: %v", err)
	}
	// an initial import has no previous snapshot
	if _, err := os.Lstat(s.PreviousDir(7)); !os.IsNotExist(err) {
		t.Fatalf("initial import must not rotate a previous snapshot, stat: %v", err)
	}
	// staging is cleaned up
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatalf("staging must be removed at finalize, stat: %v", err)
	}
}

func TestFinalizeIsIdempotent(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7

	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	// A completed finalization removes the operation directory; a re-run is
	// no longer attributable to anything and is ambiguous, never blessed by
	// digest matches alone.
	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("finalize after completion: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil {
		t.Fatal("baseline must survive re-finalize")
	}
}

func TestFinalizeRequiresSkillID(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("finalize without a Skill id: got %v, want ErrAmbiguous", err)
	}
}

func TestFinalizeBlocksOnLiveMismatch(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7

	// the live tree changes after the commit (external interference)
	if err := os.WriteFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("live mismatch: got %v, want ErrAmbiguous", err)
	}
	// candidates are preserved
	if _, err := os.Lstat(filepath.Join(s.baselineCandidateDir(1), "SKILL.md")); err != nil {
		t.Fatalf("baseline candidate must be preserved, stat: %v", err)
	}
}

func TestFinalizeReplaceRotatesPreviousSnapshot(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	op.SkillID = 7

	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	// the Baseline advances to the accepted content
	if data, err := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil || string(data) != "new content" {
		t.Fatalf("baseline after replace: %q, %v", data, err)
	}
	// the recovery slot became the single previous snapshot
	if data, err := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); err != nil || string(data) != "old content" {
		t.Fatalf("previous snapshot: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); !os.IsNotExist(err) {
		t.Fatalf("recovery slot must be consumed, stat: %v", err)
	}
}

// TestFinalizeReplaceRejectsTamperedRecovery proves the recovery slot is
// digest-proven before it becomes the previous snapshot; unproven content
// is preserved and reported as ambiguous.
func TestFinalizeReplaceRejectsTamperedRecovery(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	op.SkillID = 7
	if err := os.WriteFile(filepath.Join(s.recoveryDir(2), "SKILL.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("tampered recovery: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("tampered recovery content must be preserved")
	}
}

// TestFinalizeRotationSupersedesPreviousSnapshot proves a second replace
// rotates the new snapshot while the old one is dropped only after the new
// one is safely in place.
func TestFinalizeRotationSupersedesPreviousSnapshot(t *testing.T) {
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
	op2 := Operation{ID: 3, Slug: "alpha", Kind: KindReplace, OldDigest: second.digest, NewDigest: third.digest}
	stageAndInstall(t, s, op2, third)
	op2.SkillID = 7
	if err := s.Finalize(context.Background(), op2); err != nil {
		t.Fatal(err)
	}

	if data, err := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); err != nil || string(data) != "second" {
		t.Fatalf("previous after second replace: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.PreviousDir(7) + ".old"); !os.IsNotExist(err) {
		t.Fatalf("stale temp must be cleaned, stat: %v", err)
	}
}

// TestFinalizeRotationConvergesAfterCrash simulates a crash between moving
// the superseded snapshot aside and installing the new one: the stale temp
// that pre-exists rotation has no operation-attribution proof on restart,
// so even a byte-identical candidate is preserved with ErrAmbiguous
// instead of being drained on a fresh sample.
func TestFinalizeRotationConvergesAfterCrash(t *testing.T) {
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
	// previous now holds the first snapshot; the second replace is prepared
	// and its recovery slot holds the second content
	third := buildMaterialized(t, map[string]string{"SKILL.md": "third"})
	op2 := Operation{ID: 3, Slug: "alpha", Kind: KindReplace, OldDigest: second.digest, NewDigest: third.digest}
	stageAndInstall(t, s, op2, third)
	op2.SkillID = 7
	// crash state: previous moved aside, recovery not yet rotated
	if err := os.Rename(s.PreviousDir(7), s.PreviousDir(7)+".old"); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op2)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("pre-existing stale previous: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.PreviousDir(7) + ".old"); err != nil {
		t.Fatal("the pre-existing stale snapshot must be preserved")
	}
	if _, err := os.Lstat(s.recoveryDir(3)); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
	// the Baseline was advanced by the proven moves before rotation stopped;
	// the old Baseline is preserved as the operation's own backup
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "third" {
		t.Fatalf("Baseline after the proven moves: %q, %v", data, rerr)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.baselineBackupDir(3), "SKILL.md")); rerr != nil || string(data) != "second" {
		t.Fatalf("the moved Backup must be preserved: %q, %v", data, rerr)
	}
}

// TestFinalizeRefusesPreExistingByteIdenticalStale proves the restart rule
// even for a byte-identical stale snapshot: a previous/<id>.old that
// pre-exists rotation is preserved with ErrAmbiguous.
func TestFinalizeRefusesPreExistingByteIdenticalStale(t *testing.T) {
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
	// the second replace is prepared; a byte-identical copy of the current
	// previous snapshot sits in the stale slot before rotation starts
	third := buildMaterialized(t, map[string]string{"SKILL.md": "third"})
	op2 := Operation{ID: 3, Slug: "alpha", Kind: KindReplace, OldDigest: second.digest, NewDigest: third.digest}
	stageAndInstall(t, s, op2, third)
	op2.SkillID = 7
	if err := copyTreeForTest(s.PreviousDir(7)+".old", s.PreviousDir(7)); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op2)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical pre-existing stale: got %v, want ErrAmbiguous", err)
	}
	for _, candidate := range []string{s.PreviousDir(7), s.PreviousDir(7) + ".old", s.recoveryDir(3)} {
		if _, statErr := os.Lstat(candidate); statErr != nil {
			t.Fatalf("ambiguous candidate %q must be preserved: %v", candidate, statErr)
		}
	}
}

func TestRecoverDispatchesByCommitted(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

	// pending: restore unwinds the uncommitted install
	op.SkillID = 7
	if err := s.Recover(context.Background(), op, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("pending recovery must restore (remove) the uncommitted import")
	}

	// committed: finalize completes the import
	stageAndInstall(t, s, op, m)
	if err := s.Recover(context.Background(), op, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil {
		t.Fatal("committed recovery must finalize the baseline")
	}
}
