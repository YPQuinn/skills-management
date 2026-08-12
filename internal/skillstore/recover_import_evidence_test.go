package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreImportNoEvidenceWithLiveIsEmptyState proves a pending import
// with no operation evidence at all is the not-started state even when a
// live directory exists at the slug: PrepareRestore reports the empty
// receipt so the application can clear the intent through the abort
// protocol, and the live tree is never read, moved, or deleted here.
func TestRestoreImportNoEvidenceWithLiveIsEmptyState(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "digest"}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatalf("no-evidence import restore: %v", err)
	}
	if !receipt.Empty() {
		t.Fatalf("no-evidence import must be the empty state: %+v", receipt)
	}
	if data, rerr := os.ReadFile(filepath.Join(live, "mine.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("the live tree must be preserved: %q, %v", data, rerr)
	}
}

// TestRestoreImportPreservesByteIdenticalUnmanagedLive proves pending
// recovery never drains a live tree merely because its digest matches
// NewDigest: a byte-identical unmanaged directory that appeared before
// install is preserved, and the surviving staging (which has no persisted
// physical proof on restart) is preserved with ErrAmbiguous.
func TestRestoreImportPreservesByteIdenticalUnmanagedLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	// a byte-identical unmanaged directory appears after the intent but
	// before install (same path, same bytes, same mode)
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore with unmanaged live: got %v, want ErrAmbiguous", err)
	}
	if data, err := os.ReadFile(filepath.Join(live, "SKILL.md")); err != nil || string(data) != "content" {
		t.Fatalf("byte-identical unmanaged content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("staging must be preserved")
	}
}

// TestRestoreImportPreservesDifferentUnmanagedLive covers the same evidence
// rule for unmanaged content whose digest differs from the operation's.
func TestRestoreImportPreservesDifferentUnmanagedLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("someone else"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore with unmanaged live: got %v, want ErrAmbiguous", err)
	}
	if data, err := os.ReadFile(filepath.Join(live, "SKILL.md")); err != nil || string(data) != "someone else" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("staging must be preserved")
	}
}

// TestRestoreImportCrashBetweenCandidateAndRename simulates a crash after
// Install copied the Baseline candidate but before the staged→live rename:
// the surviving staged tree proves no install happened, but without a
// persisted physical proof (Stage-A writes no durable receipt) recovery
// preserves the staging and the candidate with ErrAmbiguous.
func TestRestoreImportCrashBetweenCandidateAndRename(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeForTest(s.baselineCandidateDir(1), m.dir); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore after the candidate copy: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may exist")
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("staging and the candidate must be preserved")
	}
}

// TestRestoreImportMissingCandidateIsAmbiguous proves a consumed staged tree
// without its digest-proven Baseline candidate cannot be attributed to the
// operation: restore refuses and preserves the live tree.
func TestRestoreImportMissingCandidateIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	if err := os.RemoveAll(s.baselineCandidateDir(1)); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("missing candidate: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil {
		t.Fatal("live content must be preserved")
	}
}

// TestRestoreImportMissingLiveAfterConsumeCompletes proves a consumed
// staged tree whose live tree vanished is still deterministic under the
// durable install proof: the install was already removed (its quarantine
// cleanup completed, or the uncommitted tree was lost) and nothing of the
// pre-import state remains to restore, so restore completes and clears the
// operation artifacts.
func TestRestoreImportMissingLiveAfterConsumeCompletes(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatalf("missing live after a proven install: %v", err)
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("operation artifacts must be cleared")
	}
}

// TestRestoreReplacePreRefusalDiscardsStaging proves a replace whose live
// tree changed before Install (a pre-mutation refusal) is preserved on
// restart: a surviving staged tree has no persisted physical proof, so
// recovery reports ErrAmbiguous and keeps the edited live content, the
// staging, and the intent.
func TestRestoreReplacePreRefusalDiscardsStaging(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	// the live tree changes after the operation was prepared
	if err := os.WriteFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	if _, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("pre-refusal restore: got %v, want ErrAmbiguous", err)
	}
	if data, err := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil || string(data) != "edited" {
		t.Fatalf("edited live content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatal("staging must be preserved")
	}
}

// TestDiscardStagingRemovesOnlyOperationContent proves the pre-install
// cleanup primitive removes the operation's staging while leaving any live
// (unmanaged) content untouched.
func TestDiscardStagingRemovesOnlyOperationContent(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "precious.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.DiscardStaging(op.ID, staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("staging must be removed")
	}
	if data, err := os.ReadFile(filepath.Join(live, "precious.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("live content must be untouched: %q, %v", data, err)
	}
	// the layout survives for the next operation
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
}
