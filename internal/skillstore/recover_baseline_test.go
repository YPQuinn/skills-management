package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizeRejectsUnexpectedExistingBaseline(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.WriteFile(filepath.Join(s.BaselineDir(7), "SKILL.md"), []byte("unexpected"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("unexpected Baseline: got %v, want ErrAmbiguous", err)
	}
	for _, candidate := range []string{s.BaselineDir(7), s.baselineCandidateDir(2), s.recoveryDir(2)} {
		if _, statErr := os.Lstat(candidate); statErr != nil {
			t.Fatalf("ambiguous candidate %q must be preserved: %v", candidate, statErr)
		}
	}
}

// TestFinalizeResumesAfterBaselineMovedAside proves a saved Baseline that
// pre-exists finalization (a crash leftover or a foreign directory) has no
// operation-attribution proof on restart: even a byte-identical candidate
// is preserved with ErrAmbiguous, because the current sample cannot prove
// this operation moved it. The same-invocation rollback path uses only the
// identity retained from the move itself.
func TestFinalizeResumesAfterBaselineMovedAside(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	opDir, err := layout.opDir(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplaceAt(layout.baselines, "7", opDir, "baseline-old"); err != nil {
		t.Fatal(err)
	}
	opDir.Close()
	layout.close()

	err = s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("pre-existing saved Baseline: got %v, want ErrAmbiguous", err)
	}
	for _, candidate := range []string{s.baselineBackupDir(2), s.baselineCandidateDir(2), s.recoveryDir(2)} {
		if _, statErr := os.Lstat(candidate); statErr != nil {
			t.Fatalf("ambiguous candidate %q must be preserved: %v", candidate, statErr)
		}
	}
	if _, err := os.Lstat(s.BaselineDir(7)); !os.IsNotExist(err) {
		t.Fatal("no Baseline may be installed")
	}
}

// TestFinalizeRefusesPreExistingByteIdenticalBackup proves the restart
// rule even for a byte-identical saved Baseline: a pre-existing
// baseline-old whose digest matches the replaced digest is still not
// attributable to the operation and is preserved with ErrAmbiguous.
func TestFinalizeRefusesPreExistingByteIdenticalBackup(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	// a byte-identical copy of the old Baseline sits in the backup slot
	// before finalization starts
	if err := copyTreeForTest(s.baselineBackupDir(2), s.BaselineDir(7)); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical pre-existing backup: got %v, want ErrAmbiguous", err)
	}
	for _, candidate := range []string{s.BaselineDir(7), s.baselineBackupDir(2), s.baselineCandidateDir(2), s.recoveryDir(2)} {
		if _, statErr := os.Lstat(candidate); statErr != nil {
			t.Fatalf("ambiguous candidate %q must be preserved: %v", candidate, statErr)
		}
	}
}
