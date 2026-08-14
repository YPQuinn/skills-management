package skillstore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestPrepareFinalizeResumesAfterStalePreviousMovedAside proves the
// finalize crash window between the previous-snapshot move and the
// recovery rotation: a fresh Store instance attributes previous/<id>.old
// through the durable witness, completes the rotation, drains the stale
// snapshot, and converges.
func TestPrepareFinalizeResumesAfterStalePreviousMovedAside(t *testing.T) {
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
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	opDir, err := layout.opDir(op2.ID)
	if err != nil {
		t.Fatal(err)
	}
	proof, _, err := readProof(opDir)
	if err != nil || proof == nil {
		t.Fatal("test setup: the install proof must exist")
	}
	// reproduce the crash state: previous/7 moved to previous/7.old with
	// its durable witness
	prevName := strconv.FormatInt(op2.SkillID, 10)
	prev, prevID, prevDigest, err := openTreeDigest(context.Background(), layout.previous, prevName)
	if err != nil {
		t.Fatal(err)
	}
	prev.Close()
	if _, err := writeTransientWitness(opDir, staleWitnessName, op2.ID, witnessStalePrev, prevID, prevDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := movePinned(context.Background(), layout, layout.previous, prevName, layout.previous, prevName+".old", prevID, prevDigest); err != nil {
		t.Fatal(err)
	}
	opDir.Close()
	layout.close()

	receipt, err := New(s.Root).PrepareFinalize(context.Background(), op2)
	if err != nil {
		t.Fatalf("resume after the stale snapshot moved aside: %v", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.PreviousDir(7), "SKILL.md")); rerr != nil || string(data) != "second" {
		t.Fatalf("the rotated previous snapshot after resume: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "previous", "7.old")); !os.IsNotExist(err) {
		t.Fatal("the witnessed stale snapshot must be drained on resume")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(3), staleWitnessName)); !os.IsNotExist(err) {
		t.Fatal("the stale witness must be removed after the drain")
	}
	if receipt.prevID == (fileID{}) {
		t.Fatalf("resumed finalize receipt must bind the rotated previous: %+v", receipt)
	}
}

// TestPrepareFinalizeResumeReceiptMatchesExpected proves the resumed
// preparation returns the receipt the proof-derived terminal state
// requires: the resume never changes the opDir, proof, live, candidate, or
// recovery identities, so the receipt equals newFinalizeReceipt computed
// from the retained proof.
func TestPrepareFinalizeResumeReceiptMatchesExpected(t *testing.T) {
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
	proof, proofFileID, err := readProof(opDir)
	if err != nil || proof == nil {
		t.Fatal("test setup: the install proof must exist")
	}
	opDirID, err := rootID(opDir)
	if err != nil {
		t.Fatal(err)
	}
	moveBaselineAside(t, s, op, proof, opDir)
	opDir.Close()
	layout.close()

	resumed, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	expected := newFinalizeReceipt(op, opDirID, proofFileID, proof, proof.candidateID)
	if !bytes.Equal(resumed.Bytes(), expected.Bytes()) {
		t.Fatalf("resumed receipt differs from the expected receipt:\n%q\n%q", resumed.Bytes(), expected.Bytes())
	}
}
