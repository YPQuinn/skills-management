package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestPrepareFinalizeRefusesCoexistingWitnessTombstone proves a canonical
// witness and its tombstone coexisting is foreign activity: both are
// preserved with ErrAmbiguous.
func TestPrepareFinalizeRefusesCoexistingWitnessTombstone(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	witnessPath := filepath.Join(opDir, baselineWitnessName)
	data, err := os.ReadFile(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(witnessPath+".del", data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("coexisting witness tombstone: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(witnessPath); err != nil {
		t.Fatal("the canonical witness must be preserved")
	}
	if _, err := os.Lstat(witnessPath + ".del"); err != nil {
		t.Fatal("the tombstone must be preserved")
	}
}

// TestPrepareFinalizeResumesWitnessRemovalFromTombstone proves a removal
// interrupted after the detach resumes from the tombstone: the fresh
// preparation proves the tombstone is the very witness (canonical,
// self-consistent, matching operation and slot), unlinks it, and completes
// the finalize.
func TestPrepareFinalizeResumesWitnessRemovalFromTombstone(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	// simulate the drained transient and the interrupted removal: the
	// backup is gone and the witness was detached into its tombstone
	if err := os.RemoveAll(filepath.Join(opDir, "baseline-old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(opDir, baselineWitnessName), filepath.Join(opDir, baselineWitnessName+".del")); err != nil {
		t.Fatal(err)
	}

	if _, err := New(s.Root).PrepareFinalize(context.Background(), op); err != nil {
		t.Fatalf("resume from the witness tombstone: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, baselineWitnessName+".del")); !os.IsNotExist(err) {
		t.Fatal("the resumed removal must unlink the tombstone")
	}
	if _, err := os.Lstat(filepath.Join(opDir, baselineWitnessName)); !os.IsNotExist(err) {
		t.Fatal("the witness must be gone")
	}
}

// TestPrepareFinalizeRefusesMalformedWitnessTombstone proves a malformed
// tombstone is preserved with ErrAmbiguous: no object is removed on a
// fresh sample.
func TestPrepareFinalizeRefusesMalformedWitnessTombstone(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	if err := os.RemoveAll(filepath.Join(opDir, "baseline-old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(opDir, baselineWitnessName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opDir, baselineWitnessName+".del"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("malformed witness tombstone: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(opDir, baselineWitnessName+".del")); rerr != nil || string(data) != "garbage" {
		t.Fatalf("the malformed tombstone must be preserved: %q, %v", data, rerr)
	}
}

// TestPrepareFinalizeResumesStaleWitnessRemovalFromTombstone proves an
// interrupted stale-witness removal resumes from the tombstone: the stale
// snapshot was already drained and the witness was detached, so a fresh
// preparation proves the tombstone is the very witness, unlinks it, and
// completes the finalize.
func TestPrepareFinalizeResumesStaleWitnessRemovalFromTombstone(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	// simulate the drained transient and the interrupted removal
	if err := os.RemoveAll(filepath.Join(s.InternalDir(), "previous", "7.old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(opDir, staleWitnessName), filepath.Join(opDir, staleWitnessName+".del")); err != nil {
		t.Fatal(err)
	}

	if _, err := New(s.Root).PrepareFinalize(context.Background(), op); err != nil {
		t.Fatalf("resume the stale witness removal from the tombstone: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, staleWitnessName+".del")); !os.IsNotExist(err) {
		t.Fatal("the resumed removal must unlink the tombstone")
	}
	if _, err := os.Lstat(filepath.Join(opDir, staleWitnessName)); !os.IsNotExist(err) {
		t.Fatal("the witness must be gone")
	}
}

// TestPrepareFinalizeRefusesForeignStaleWitnessReplacement proves a
// foreign file taking the stale witness name is preserved with
// ErrAmbiguous: no object is removed on a fresh sample.
func TestPrepareFinalizeRefusesForeignStaleWitnessReplacement(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedStaleState(t, s)
	witnessPath := filepath.Join(opDir, staleWitnessName)
	if err := os.Remove(witnessPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(witnessPath, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign stale witness replacement: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(witnessPath); rerr != nil || string(data) != "foreign" {
		t.Fatalf("the foreign replacement must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "previous", "7.old")); err != nil {
		t.Fatal("the stale snapshot must be preserved")
	}
}
