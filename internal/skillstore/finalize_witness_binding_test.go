package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"skillctl/internal/source"
)

// writeRawWitness writes a manually-constructed witness at path whose self
// identity field is filled from the created file's actual identity, so a
// test can vary the operation, action, transient identity, or digest while
// keeping the file self-consistent (or deliberately not).
func writeRawWitness(t *testing.T, path string, opID int64, action witnessAction, id fileID, digest string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	self, err := fileIDOf(info)
	if err != nil {
		t.Fatal(err)
	}
	w := transientWitness{opID: opID, action: action, selfID: self, id: id, digest: digest}
	if err := os.WriteFile(path, w.bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// replacedReplaceState builds a committed replace whose preparation crashed
// between the Baseline backup move and the candidate move, returning the
// op and the opDir path for tampering.
func replacedReplaceState(t *testing.T, s Store) (Operation, string) {
	t.Helper()
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
	return op, s.opDir(op.ID)
}

// TestPrepareFinalizeRefusesCopiedByteIdenticalWitness proves a witness
// whose content was copied into a fresh file is refused: the recorded self
// identity no longer matches the file's physical identity, so a copied
// witness never authorizes the transient drain.
func TestPrepareFinalizeRefusesCopiedByteIdenticalWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	witnessPath := filepath.Join(opDir, baselineWitnessName)
	replaceFileWithSiblingCopy(t, witnessPath)

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("copied witness: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, "baseline-old")); err != nil {
		t.Fatal("the backup must be preserved")
	}
	if _, err := os.Lstat(witnessPath); err != nil {
		t.Fatal("the copied witness must be preserved")
	}
}

func TestPrepareFinalizeRefusesWrongWitnessBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string)
	}{
		{
			name: "wrong operation",
			mutate: func(p string) {
				writeRawWitness(t, p, 999, witnessBaseline, fileID{dev: 5, ino: 5}, "digest")
			},
		},
		{
			name: "wrong action",
			mutate: func(p string) {
				writeRawWitness(t, p, 2, witnessStalePrev, fileID{dev: 5, ino: 5}, "digest")
			},
		},
		{
			name: "wrong transient identity",
			mutate: func(p string) {
				writeRawWitness(t, p, 2, witnessBaseline, fileID{dev: 5, ino: 5}, "digest")
			},
		},
		{
			name: "wrong digest",
			mutate: func(p string) {
				writeRawWitness(t, p, 2, witnessBaseline, fileID{dev: 9, ino: 9}, "wrong-digest")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t)
			op, opDir := replacedReplaceState(t, s)
			tt.mutate(filepath.Join(opDir, baselineWitnessName))

			_, err := New(s.Root).PrepareFinalize(context.Background(), op)
			if !errors.Is(err, ErrAmbiguous) {
				t.Fatalf("%s: got %v, want ErrAmbiguous", tt.name, err)
			}
			if _, err := os.Lstat(filepath.Join(opDir, "baseline-old")); err != nil {
				t.Fatal("the backup must be preserved")
			}
			if _, err := os.Lstat(filepath.Join(opDir, baselineWitnessName)); err != nil {
				t.Fatal("the foreign witness must be preserved")
			}
		})
	}
}

// TestPrepareFinalizeRefusesBaselineDigestNotOldDigest proves the Baseline
// witness additionally binds the old digest: a self-consistent witness
// whose digest equals the current backup content but not op.OldDigest is
// refused, so an edited backup can never be drained under a forged
// progress record.
func TestPrepareFinalizeRefusesBaselineDigestNotOldDigest(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	backup := filepath.Join(opDir, "baseline-old")
	if err := os.WriteFile(filepath.Join(backup, "injected.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(backup)
	if err != nil {
		t.Fatal(err)
	}
	newDigest, err := source.TreeDigestRoot(context.Background(), root)
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(backup)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fileIDOf(info)
	if err != nil {
		t.Fatal(err)
	}
	writeRawWitness(t, filepath.Join(opDir, baselineWitnessName), op.ID, witnessBaseline, id, newDigest)

	_, err = New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Baseline digest not equal to the old digest: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(backup, "injected.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the edited backup must be preserved: %q, %v", data, rerr)
	}
}

// TestPrepareFinalizeRefusesSymlinkWitness proves a symlink at the witness
// name is never accepted or followed: the preparation is refused and the
// symlink is preserved.
func TestPrepareFinalizeRefusesSymlinkWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	witnessPath := filepath.Join(opDir, baselineWitnessName)
	if err := os.Remove(witnessPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(opDir, "baseline-old"), witnessPath); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("symlink witness: got %v, want ErrAmbiguous", err)
	}
	if target, rerr := os.Readlink(witnessPath); rerr != nil || target == "" {
		t.Fatalf("the symlink witness must be preserved: %q, %v", target, rerr)
	}
}

// TestPrepareFinalizeRefusesHardlinkedWitness proves a witness with more
// than one link cannot be proven exclusive and is refused.
func TestPrepareFinalizeRefusesHardlinkedWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	witnessPath := filepath.Join(opDir, baselineWitnessName)
	if err := os.Link(witnessPath, witnessPath+".extra"); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(witnessPath + ".extra")

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("hardlinked witness: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(opDir, "baseline-old")); err != nil {
		t.Fatal("the backup must be preserved")
	}
}

// TestPrepareFinalizeRefusesFIFOWitness proves a special node (a FIFO) at
// the witness name is never accepted: the file policy refuses it, the
// preparation is ErrAmbiguous, and the FIFO is preserved.
func TestPrepareFinalizeRefusesFIFOWitness(t *testing.T) {
	s := newStore(t)
	op, opDir := replacedReplaceState(t, s)
	witnessPath := filepath.Join(opDir, baselineWitnessName)
	if err := os.Remove(witnessPath); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(witnessPath, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("FIFO witness: got %v, want ErrAmbiguous", err)
	}
	if info, rerr := os.Lstat(witnessPath); rerr != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("the FIFO witness must be preserved: %v, %v", info, rerr)
	}
}
