package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// moveBaselineAside reproduces the exact crash state of installBaseline
// between the backup move and the candidate move: the old Baseline is
// moved into opDir/baseline-old with a durable witness written by the same
// helpers the preparation uses.
func moveBaselineAside(t *testing.T, s Store, op Operation, proof *installProof, opDir *os.Root) {
	t.Helper()
	ctx := context.Background()
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	baseName := strconv.FormatInt(op.SkillID, 10)
	base, baseID, baseDigest, err := openTreeDigest(ctx, layout.baselines, baseName)
	if err != nil {
		t.Fatal(err)
	}
	base.Close()
	if baseDigest != op.OldDigest {
		t.Fatalf("test setup: the Baseline digest %s must equal the old digest", baseDigest)
	}
	if _, err := writeTransientWitness(opDir, baselineWitnessName, op.ID, witnessBaseline, baseID, baseDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := movePinned(ctx, layout, layout.baselines, baseName, opDir, "baseline-old", baseID, op.OldDigest); err != nil {
		t.Fatal(err)
	}
}

// TestPrepareFinalizeResumesAfterBaselineMovedAside proves the finalize
// crash window between the Baseline backup move and the candidate move: a
// fresh Store instance attributes baseline-old through the durable witness,
// completes the candidate move and the rotation, drains the backup, and
// returns the same effective receipt a full preparation returns.
func TestPrepareFinalizeResumesAfterBaselineMovedAside(t *testing.T) {
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
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	proof, _, err := readProof(opDir)
	if err != nil || proof == nil {
		t.Fatal("test setup: the install proof must exist")
	}
	moveBaselineAside(t, s, op, proof, opDir)
	opDir.Close()
	layout.close()

	// a fresh Store resumes the interrupted preparation
	receipt, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatalf("resume after the Baseline moved aside: %v", err)
	}
	if receipt.action != actionFinalize || receipt.opDirID == (fileID{}) || receipt.baseID == (fileID{}) {
		t.Fatalf("resumed finalize receipt: %+v", receipt)
	}
	// the new Baseline is installed and the old one is drained
	if data, rerr := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); rerr != nil || string(data) != "new content" {
		t.Fatalf("Baseline after resume: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(2), "baseline-old")); !os.IsNotExist(err) {
		t.Fatal("the witnessed backup must be drained on resume")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(2), baselineWitnessName)); !os.IsNotExist(err) {
		t.Fatal("the witness must be removed after the backup was drained")
	}
	// the opDir holds only the proof, so the receipt-bound cleanup accepts it
	op.Phase = PhaseFinalized
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(context.Background(), op, parsed); err != nil {
		t.Fatal(err)
	}
}

// TestPrepareFinalizeStaleWitnessIsRemoved proves a witness whose transient
// was already drained by a previous preparation is removed on the next
// preparation, so the operation directory returns to the proof-only state
// the receipt-bound cleanup requires.
func TestPrepareFinalizeStaleWitnessIsRemoved(t *testing.T) {
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
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	baseName := strconv.FormatInt(op.SkillID, 10)
	base, baseID, baseDigest, err := openTreeDigest(context.Background(), layout.baselines, baseName)
	if err != nil {
		t.Fatal(err)
	}
	base.Close()
	// write a witness with no transient behind it (a previous preparation
	// drained the backup but crashed before removing the witness)
	if _, err := writeTransientWitness(opDir, baselineWitnessName, op.ID, witnessBaseline, baseID, baseDigest); err != nil {
		t.Fatal(err)
	}
	opDir.Close()
	layout.close()

	if _, err := New(s.Root).PrepareFinalize(context.Background(), op); err != nil {
		t.Fatalf("prepare with a stale witness: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(2), baselineWitnessName)); !os.IsNotExist(err) {
		t.Fatal("the stale witness must be removed")
	}
}

// TestPrepareFinalizeRefusesForeignTransientWithWitness proves the witness
// never relaxes the transient identity: a byte-identical foreign tree
// substituted into the witnessed baseline-old slot is refused with
// ErrAmbiguous and preserved together with the witness.
func TestPrepareFinalizeRefusesForeignTransientWithWitness(t *testing.T) {
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
	opDir, err := layout.opDir(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	proof, _, err := readProof(opDir)
	if err != nil || proof == nil {
		t.Fatal("test setup: the install proof must exist")
	}
	moveBaselineAside(t, s, op, proof, opDir)
	// a byte-identical foreign tree takes the witnessed slot
	backup := filepath.Join(s.opDir(op.ID), "baseline-old")
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
	opDir.Close()
	layout.close()

	_, err = New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign witnessed transient: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "SKILL.md")); err != nil {
		t.Fatal("the byte-identical foreign backup must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(real, "SKILL.md")); err != nil {
		t.Fatal("the moved-aside Baseline must survive beside the foreign one")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), baselineWitnessName)); err != nil {
		t.Fatal("the witness must be preserved with the foreign transient")
	}
}
