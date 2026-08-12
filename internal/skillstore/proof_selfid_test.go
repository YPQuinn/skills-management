package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// copyProofBytes replaces the operation's install proof with a
// byte-identical file in a fresh inode: the original bytes are copied to a
// new file while the original still exists, and the copy is renamed over
// the original, so the new inode is guaranteed distinct.
func copyProofBytes(t *testing.T, proofPath string) {
	t.Helper()
	data, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal(err)
	}
	tmp := proofPath + ".copy"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, proofPath); err != nil {
		t.Fatal(err)
	}
}

// assertForeignProofPreserved asserts the operation directory and the
// byte-identical foreign proof survive beside the original proof content.
func assertForeignProofPreserved(t *testing.T, s Store, opID int64) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(s.opDir(opID), "proof")); err != nil {
		t.Fatal("the byte-identical foreign proof must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(opID), "baseline")); err != nil {
		t.Fatal("the Baseline candidate must be preserved")
	}
}

// TestFinalizeRejectsByteIdenticalForeignProof proves the durable proof
// binds its own physical identity: a byte-identical proof copied into a
// fresh inode after Install is refused by committed finalization with
// ErrAmbiguous and preserved, so the operation can never be finalized or
// cleaned through a proof this operation did not create.
func TestFinalizeRejectsByteIdenticalForeignProof(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	copyProofBytes(t, filepath.Join(s.opDir(1), "proof"))

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign proof: got %v, want ErrAmbiguous", err)
	}
	assertForeignProofPreserved(t, s, 1)
	if _, err := os.Lstat(liveDir(t, s, "alpha")); err != nil {
		t.Fatal("the live tree must be preserved")
	}
}

// TestRestoreImportRejectsByteIdenticalForeignProof proves the same
// binding for a pending import restore: a copied proof never authorizes
// the restore's quarantine/candidate drain.
func TestRestoreImportRejectsByteIdenticalForeignProof(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	copyProofBytes(t, filepath.Join(s.opDir(1), "proof"))

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign proof on import restore: got %v, want ErrAmbiguous", err)
	}
	assertForeignProofPreserved(t, s, 1)
	if _, err := os.Lstat(liveDir(t, s, "alpha")); err != nil {
		t.Fatal("the installed live tree must be preserved")
	}
}

// TestRestoreReplaceRejectsByteIdenticalForeignProof proves the same
// binding for a pending replace restore: the copied proof, the live tree,
// and the recovery slot are all preserved.
func TestRestoreReplaceRejectsByteIdenticalForeignProof(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	copyProofBytes(t, filepath.Join(s.opDir(2), "proof"))

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("byte-identical foreign proof on replace restore: got %v, want ErrAmbiguous", err)
	}
	assertForeignProofPreserved(t, s, 2)
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); err != nil {
		t.Fatal("the installed live tree must be preserved")
	}
}

// TestPrepareFinalizeRejectsMalformedProof proves a proof that no longer
// parses exactly is foreign and preserved with ErrAmbiguous.
func TestPrepareFinalizeRejectsMalformedProof(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	if err := os.WriteFile(filepath.Join(s.opDir(1), "proof"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("malformed proof: got %v, want ErrAmbiguous", err)
	}
	assertForeignProofPreserved(t, s, 1)
}
