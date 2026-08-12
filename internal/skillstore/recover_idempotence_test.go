package skillstore

import (
	"context"
	"os"
	"testing"
)

// TestRestoreImportIdempotentAfterCleanup proves pending-import restore is
// idempotent when a previous restore already cleaned the filesystem but the
// journal row was not deleted (a transient intent-deletion failure): the
// second restore sees no live tree, no staged tree, and no Baseline
// candidate and succeeds, so the next recovery can delete the intent.
func TestRestoreImportIdempotentAfterCleanup(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("first restore must remove the installed live tree")
	}
	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("second restore after cleanup: %v", err)
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("restore must not leave operation artifacts")
	}
}

// TestRestoreImportNeverStartedSucceeds proves a pending intent persisted
// right before a crash, with no staging, candidate, or live tree, is the
// not-started state: restore succeeds and the journal row can be deleted.
func TestRestoreImportNeverStartedSucceeds(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "digest"}
	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("not-started restore: %v", err)
	}
}
