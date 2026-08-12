package skillstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// preparedImport builds a committed import whose semantic finalization is
// prepared, returning the operation, the receipt, and the opDir path.
func preparedImport(t *testing.T, s Store) (Operation, CleanupReceipt) {
	t.Helper()
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	receipt, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	return op, receipt
}

func TestPrepareFinalizeRetainsEvidenceUntilReceipt(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7

	receipt, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	// the semantic finalization completed: the Baseline is installed
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil {
		t.Fatalf("Baseline must be installed by preparation: %v", err)
	}
	// the exact proof/opDir evidence is retained until the receipt persists
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatalf("the install proof must survive preparation: %v", err)
	}
	if receipt.opDirID == (fileID{}) || receipt.proofID == (fileID{}) ||
		receipt.liveID == (fileID{}) || receipt.baseID == (fileID{}) || receipt.prevID != (fileID{}) {
		t.Fatalf("finalize receipt must bind opDir, proof, live, and Baseline identities: %+v", receipt)
	}
	if receipt.action != actionFinalize {
		t.Fatalf("receipt action: %q", receipt.action)
	}
}

func TestPrepareFinalizeIsIdempotent(t *testing.T) {
	s := newStore(t)
	op, first := preparedImport(t, s)
	// a fresh Store instance re-prepares the same terminal state and must
	// return the same effective receipt
	again, err := New(s.Root).PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), again.Bytes()) {
		t.Fatalf("re-prepared receipt differs:\n%q\n%q", first.Bytes(), again.Bytes())
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("preparation must keep retaining the evidence")
	}
}

func TestPrepareRestoreImportRetainsEvidence(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	// the semantic restore completed: the installed live tree is gone
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("preparation must remove the uncommitted live tree")
	}
	// the candidate was drained and the evidence is retained
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "baseline")); !os.IsNotExist(err) {
		t.Fatal("preparation must drain the Baseline candidate")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatalf("the install proof must survive preparation: %v", err)
	}
	if receipt.action != actionRestore || receipt.liveID != (fileID{}) {
		t.Fatalf("import restore receipt: %+v", receipt)
	}
}

func TestPrepareRestoreImportConvergesAfterCandidateDrained(t *testing.T) {
	s := newStore(t)
	op, _ := preparedRestoreImport(t, s)
	// a fresh Store re-prepares the already-restored state (candidate
	// drained, live absent) and returns the same effective receipt
	receipt, err := New(s.Root).PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must still be retained")
	}
	if receipt.action != actionRestore {
		t.Fatalf("receipt action: %q", receipt.action)
	}
}

func preparedRestoreImport(t *testing.T, s Store) (Operation, CleanupReceipt) {
	t.Helper()
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	return op, receipt
}

func TestPrepareRestoreReplaceRetainsEvidence(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	// the old content is back live and the recovery slot is consumed
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("live after restore prepare: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.recoveryDir(op.ID)); !os.IsNotExist(err) {
		t.Fatal("the recovery slot must be consumed by preparation")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatalf("the install proof must survive preparation: %v", err)
	}
	if receipt.liveID == (fileID{}) {
		t.Fatalf("replace restore receipt must bind the recovered live identity: %+v", receipt)
	}
}

// TestPrepareRestoreReplaceConvergesAfterRecoveryMovedBack proves the
// crash-window resume: a previous preparation already moved the recovery
// slot back to live before the receipt persisted. Re-preparation must
// resume only when the live tree's physical identity equals the proof's
// recovery identity — never from the old digest alone — and return the
// same effective receipt.
func TestPrepareRestoreReplaceConvergesAfterRecoveryMovedBack(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	first, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	// a second prepare on a fresh Store instance sees the already-restored
	// state (live = recovery identity, slot gone)
	again, err := New(s.Root).PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), again.Bytes()) {
		t.Fatalf("re-prepared replace restore receipt differs:\n%q\n%q", first.Bytes(), again.Bytes())
	}
	// the old digest alone never authorizes the resume: a live tree that
	// does not carry the proof's recovery identity is preserved
	if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	if err := copyTreeForTest(foreign, old.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(foreign, liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}
	_, err = New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("digest-only resume: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil {
		t.Fatal("the foreign live tree must be preserved")
	}
}

// TestPrepareRestoreRefusesMissingProof proves a pending operation whose
// evidence directory lost its install proof is unprovable and preserved;
// only the terminal receipt row path may clean evidence without a proof.
func TestPrepareRestoreRefusesMissingProof(t *testing.T) {
	s := newStore(t)
	op, _ := preparedRestoreImport(t, s)
	if err := os.Remove(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal(err)
	}
	_, err := New(s.Root).PrepareRestore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("prepare without the proof: got %v, want ErrAmbiguous", err)
	}
}

// TestParseCleanupReceiptRejectsNonCanonicalEncoding proves the fixed
// encoding is strictly canonical: a payload that parses to the same fields
// but is not byte-identical to the canonical Bytes() form (missing final
// newline, leading/trailing whitespace, duplicated spacing, tab-separated
// identities, or leading-zero numbers) is rejected, so a persisted row can
// only ever carry the exact bytes the CAS deletion compares.
func TestParseCleanupReceiptRejectsNonCanonicalEncoding(t *testing.T) {
	canonical := []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n")
	variants := map[string][]byte{
		"missing final newline": []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0"),
		"leading newline":       []byte("\n1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n"),
		"trailing blank line":   []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n\n"),
		"trailing spaces":       []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n "),
		"double spaces":         []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1  1\n1 1\n1 1\n1 1\n0 0\n"),
		"tab separator":         []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1\t1\n1 1\n1 1\n1 1\n0 0\n"),
		"leading zero id":       []byte("1\nfinalize\n01\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n"),
		"leading zero identity": []byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n01 1\n1 1\n1 1\n1 1\n0 0\n"),
	}
	for name, data := range variants {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCleanupReceipt(data); err == nil {
				t.Fatalf("non-canonical encoding %q must be rejected", data)
			}
		})
	}
	// the canonical bytes themselves still parse and re-encode identically
	if got, err := ParseCleanupReceipt(canonical); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got.Bytes(), canonical) {
		t.Fatalf("canonical encoding must round-trip: %q", got.Bytes())
	}
}

func TestCleanupReceiptRoundTripAndStrictParse(t *testing.T) {
	s := newStore(t)
	_, receipt := preparedImport(t, s)
	if got, err := ParseCleanupReceipt(receipt.Bytes()); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got.Bytes(), receipt.Bytes()) {
		t.Fatal("parsed receipt must re-encode identically")
	}

	for _, bad := range [][]byte{
		nil,
		{},
		[]byte("1\nfinalize\n1\n"),
		[]byte("2\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n"),
		[]byte("1\nsync\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n"),
		[]byte("1\nfinalize\n1\n7\nalpha\nsync\n\nnew\n1 1\n1 1\n1 1\n1 1\n0 0\n"),
		// a finalize import receipt may not bind a previous identity
		[]byte("1\nfinalize\n1\n7\nalpha\nimport\n\nnew\n1 1\n1 1\n1 1\n1 1\n2 2\n"),
		// a restore receipt may not bind Baseline or previous identities
		[]byte("1\nrestore\n1\n7\nalpha\nreplace\nold\nnew\n1 1\n1 1\n1 1\n2 2\n0 0\n"),
	} {
		if _, err := ParseCleanupReceipt(bad); err == nil {
			t.Fatalf("malformed receipt %q: want error", bad)
		}
	}
	// a valid import-restore receipt parses and round-trips
	valid := []byte("1\nrestore\n1\n0\nalpha\nimport\n\nnew\n1 1\n1 1\n0 0\n0 0\n0 0\n")
	if got, err := ParseCleanupReceipt(valid); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got.Bytes(), valid) {
		t.Fatalf("valid restore receipt must re-encode identically: %q", got.Bytes())
	}
}
