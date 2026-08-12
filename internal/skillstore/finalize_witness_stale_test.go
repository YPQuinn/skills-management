package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// replacedStaleState builds a committed replace #2 whose preparation
// crashed between the stale-snapshot move and the recovery rotation:
// previous/<id> sits in previous/<id>.old with its durable witness.
func replacedStaleState(t *testing.T, s Store) (Operation, string) {
	t.Helper()
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
	return op2, s.opDir(op2.ID)
}

// TestPrepareFinalizeRefusesCopiedStaleWitness proves the stale-previous
// slot also refuses a byte-identical witness copied into a fresh inode:
// the recorded self identity no longer matches, and the stale snapshot and
// the copied witness are preserved.
func TestPrepareFinalizeRefusesCopiedStaleWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	witnessPath := filepath.Join(opDir, staleWitnessName)
	data, err := os.ReadFile(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(witnessPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(witnessPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("copied stale witness: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "previous", "7.old")); err != nil {
		t.Fatal("the stale snapshot must be preserved")
	}
	if _, err := os.Lstat(witnessPath); err != nil {
		t.Fatal("the copied witness must be preserved")
	}
}

// TestPrepareFinalizeRefusesWrongStaleWitnessAction proves the action is
// part of the canonical binding: a baseline-slot witness at the stale
// witness name is refused, so a witness can never authorize a different
// transient slot.
func TestPrepareFinalizeRefusesWrongStaleWitnessAction(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	writeRawWitness(t, filepath.Join(opDir, staleWitnessName), op.ID, witnessBaseline, fileID{dev: 5, ino: 5}, "digest")

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("wrong stale witness action: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "previous", "7.old")); err != nil {
		t.Fatal("the stale snapshot must be preserved")
	}
}

// TestPrepareFinalizeRefusesStaleWitnessDigestMismatch proves the stale
// snapshot is drained only when its digest still matches the witnessed
// digest: a self-consistent witness whose digest no longer matches the
// transient is refused and everything is preserved.
func TestPrepareFinalizeRefusesStaleWitnessDigestMismatch(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	// make the stale snapshot's digest diverge from the witness
	if err := os.WriteFile(filepath.Join(s.InternalDir(), "previous", "7.old", "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a self-consistent witness (correct self identity) recording the old
	// transient identity with a digest that no longer matches
	stale := filepath.Join(s.InternalDir(), "previous", "7.old")
	info, err := os.Lstat(stale)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fileIDOf(info)
	if err != nil {
		t.Fatal(err)
	}
	writeRawWitness(t, filepath.Join(opDir, staleWitnessName), op.ID, witnessStalePrev, id, "wrong-digest")

	_, err = New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("stale witness digest mismatch: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(stale, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the edited stale snapshot must be preserved: %q, %v", data, rerr)
	}
}

// TestPrepareFinalizeRefusesFIFOStaleWitness proves the stale-previous
// slot also refuses a special node at the witness name.
func TestPrepareFinalizeRefusesFIFOStaleWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	witnessPath := filepath.Join(opDir, staleWitnessName)
	if err := os.Remove(witnessPath); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(witnessPath, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("FIFO stale witness: got %v, want ErrAmbiguous", err)
	}
	if info, rerr := os.Lstat(witnessPath); rerr != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("the FIFO stale witness must be preserved: %v, %v", info, rerr)
	}
}
