package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreImportBeforeInstall(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}

	// A surviving staged tree has no persisted physical proof on restart
	// (Stage-A writes no durable receipt), so recovery preserves the
	// staging and the intent instead of draining a fresh sample.
	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore of staged-only import: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("staging must be preserved")
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("restore before install must not leave a live tree")
	}
}

func TestRestoreImportAfterInstall(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	// the installed-but-uncommitted live tree is removed
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("uncommitted import must be removed by restore")
	}
	if _, err := os.Lstat(s.opDir(1)); !os.IsNotExist(err) {
		t.Fatal("staging must be cleaned by restore")
	}
}

func TestRestoreImportAmbiguousPreservesEverything(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	// the live tree no longer matches the operation's digest
	if err := os.WriteFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md"), []byte("someone else"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous import: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil {
		t.Fatal("ambiguous live content must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.baselineCandidateDir(1), "SKILL.md")); err != nil {
		t.Fatal("ambiguous candidates must be preserved")
	}
}

func TestRestoreReplaceBeforeMove(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	if _, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false); err != nil {
		t.Fatal(err)
	}

	// Surviving staging has no persisted physical proof on restart, so the
	// staging, the live tree, and the intent are all preserved.
	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore of staged-only replace: got %v, want ErrAmbiguous", err)
	}
	if data, err := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil || string(data) != "old content" {
		t.Fatalf("live content: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatal("staging must be preserved")
	}
}

func TestRestoreReplaceBetweenMoveAndInstall(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	if _, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false); err != nil {
		t.Fatal(err)
	}
	// simulate the crash window: live moved to recovery, staged not yet installed
	if err := os.Rename(liveDir(t, s, "alpha"), s.recoveryDir(2)); err != nil {
		t.Fatal(err)
	}

	// Surviving staging has no persisted physical proof on restart, so the
	// recovery slot and the staging are both preserved.
	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore between move and install: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may appear")
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("recovery slot must be preserved")
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatal("staging must be preserved")
	}
}

func TestRestoreReplaceAfterInstall(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)

	if err := s.Restore(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	// the old managed content is restored from recovery
	if data, err := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); err != nil || string(data) != "old content" {
		t.Fatalf("restored content: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); !os.IsNotExist(err) {
		t.Fatal("recovery slot must be consumed by restore")
	}
	if _, err := os.Lstat(s.opDir(2)); !os.IsNotExist(err) {
		t.Fatal("staging must be cleaned by restore")
	}
}

// TestRestoreReplaceRejectsTamperedRecovery proves the recovery slot is
// digest-proven before it is restored to the live path.
func TestRestoreReplaceRejectsTamperedRecovery(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.WriteFile(filepath.Join(s.recoveryDir(2), "SKILL.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("tampered recovery: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("tampered recovery content must be preserved")
	}
}

func TestRestoreReplaceAmbiguous(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	// contradictory state: the live tree still holds the old content while a
	// recovery slot also exists, which the move-then-install sequence can
	// never produce
	recovery := s.recoveryDir(2)
	if err := copyTreeForTest(recovery, liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("contradictory replace state: got %v, want ErrAmbiguous", err)
	}
	// both candidates are preserved
	if _, err := os.Lstat(liveDir(t, s, "alpha")); err != nil {
		t.Fatal("live content must be preserved")
	}
	if _, err := os.Lstat(recovery); err != nil {
		t.Fatal("recovery content must be preserved")
	}
}

// TestRestoreReplaceMissingEverythingIsEmptyState proves a pending replace
// with no operation evidence at all — no live tree, no recovery slot, no
// staging — is the not-started state: Restore reports the empty receipt so
// the application can clear the intent through the abort protocol without
// ever claiming the live tree.
func TestRestoreReplaceMissingEverythingIsEmptyState(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	// neither the live tree nor the recovery slot exists
	if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
		t.Fatal(err)
	}

	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatalf("missing live and recovery: %v", err)
	}
	if !receipt.Empty() {
		t.Fatalf("no-evidence replace must be the empty state: %+v", receipt)
	}
}

func TestRestoreUnknownKind(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "alpha", Kind: "rebind"}
	err := s.Restore(context.Background(), op)
	if err == nil || !strings.Contains(err.Error(), "unknown operation kind") {
		t.Fatalf("unknown kind restore: got %v", err)
	}
}

// copyTreeForTest duplicates one directory tree into another for test state
// construction.
func copyTreeForTest(dst, src string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
