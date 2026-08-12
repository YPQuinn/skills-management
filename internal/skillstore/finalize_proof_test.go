package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/domain"
)

// TestFinalizeRejectsByteIdenticalForeignLive proves the committed
// finalization binds the live tree to the proof's LiveID: a byte-identical
// foreign live tree installed after the commit is refused by physical
// identity even though its digest matches, and is preserved.
func TestFinalizeRejectsByteIdenticalForeignLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
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

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign live: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(live, "SKILL.md")); err != nil {
		t.Fatal("the foreign live tree must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(real, "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive")
	}
}

// TestFinalizeRejectsByteIdenticalForeignCandidate proves the Baseline
// candidate must carry the proof's CandidateID: a byte-identical foreign
// candidate swapped in after the install is refused and preserved.
func TestFinalizeRejectsByteIdenticalForeignCandidate(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	cand := s.baselineCandidateDir(1)
	real := cand + ".real"
	if err := os.Rename(cand, real); err != nil {
		t.Fatal(err)
	}
	swap := t.TempDir()
	if err := copyTreeForTest(swap, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, cand); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign candidate: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(cand, "SKILL.md")); err != nil {
		t.Fatal("the foreign candidate must be preserved")
	}
}

// TestFinalizeRejectsByteIdenticalForeignRecovery proves the recovery slot
// must carry the proof's RecoveryID: a byte-identical foreign recovery slot
// swapped in after the install is refused and preserved.
func TestFinalizeRejectsByteIdenticalForeignRecovery(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	rec := s.recoveryDir(2)
	real := rec + ".real"
	if err := os.Rename(rec, real); err != nil {
		t.Fatal(err)
	}
	swap := t.TempDir()
	if err := copyTreeForTest(swap, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, rec); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign recovery: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(rec, "SKILL.md")); err != nil {
		t.Fatal("the foreign recovery slot must be preserved")
	}
}

// TestFinalizeCommittedOpDirMissingIsAmbiguous proves a committed operation
// whose staging vanished (a crash leftover or foreign deletion) is never
// recreated: Finalize reports ErrAmbiguous and the journal stays committed.
func TestFinalizeCommittedOpDirMissingIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	if err := os.RemoveAll(s.opDir(1)); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("missing operation directory: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("no Baseline may be installed without operation evidence")
	}
}

// TestFinalizeCommittedProofMissingIsAmbiguous proves an operation directory
// without its durable proof is unprovable: Finalize reports ErrAmbiguous and
// the live tree and candidate stay untouched.
func TestFinalizeCommittedProofMissingIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	if err := os.RemoveAll(filepath.Join(s.opDir(1), "proof")); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("missing proof: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.baselineCandidateDir(1), "SKILL.md")); err != nil {
		t.Fatal("the candidate must be preserved")
	}
}

// TestDiscardStagingAfterInstallFailureRemovesCandidate proves the opaque
// discard after a pre-mutation Install failure removes the Baseline
// candidate this invocation created together with the staged tree, using
// the creation manifest — never a fresh sample. It drives the exact helpers
// Install uses so the manifest carries the candidate entries.
func TestDiscardStagingAfterInstallFailureRemovesCandidate(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := openLayoutDir(opDir, "tree")
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	cand, candID, err := createPinnedDirExclusive(opDir, "baseline", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer cand.Close()
	staged.ownership.record("1/baseline", candID)
	if err := copyStagedTree(context.Background(), tree, ".", cand, "1/baseline", &domain.Guard{AllowLarge: true}, staged.ownership); err != nil {
		t.Fatal(err)
	}

	if err := s.DiscardStaging(op.ID, staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("the operation directory must be removed")
	}
}
