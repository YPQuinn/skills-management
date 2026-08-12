package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreReplaceStagedSurvivesPreservesByteIdenticalLive is the
// critical replace-race regression: Install moved the old live tree to the
// recovery slot, but an independently created byte-identical tree appeared
// at live before the staged→live rename, so staging survived and the
// no-replace rename refused. The surviving staged tree proves this
// operation never installed the live tree, so restore must preserve live,
// staged, recovery, and the Baseline candidate and report ErrAmbiguous.
func TestRestoreReplaceStagedSurvivesPreservesByteIdenticalLive(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	if _, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false); err != nil {
		t.Fatal(err)
	}
	// Install copies the Baseline candidate before the rename...
	if err := copyTreeForTest(s.baselineCandidateDir(2), replacement.dir); err != nil {
		t.Fatal(err)
	}
	// ...moves the old live tree to the recovery slot...
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	live := liveDir(t, s, "alpha")
	if err := renameNoReplaceAt(layout.store, "alpha", layout.recovery, "2"); err != nil {
		t.Fatal(err)
	}
	// ...and an external byte-identical tree appears at live, making the
	// staged→live no-replace rename fail with staging intact.
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("replace race: got %v, want ErrAmbiguous", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(live, "SKILL.md")); readErr != nil || string(data) != "new content" {
		t.Fatalf("live candidate must be preserved: %q, %v", data, readErr)
	}
	if _, err := os.Lstat(s.stagedTreeDir(2)); err != nil {
		t.Fatal("staged tree must be preserved")
	}
	if data, err := os.ReadFile(filepath.Join(s.recoveryDir(2), "SKILL.md")); err != nil || string(data) != "old content" {
		t.Fatalf("recovery candidate must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(s.baselineCandidateDir(2), "SKILL.md")); err != nil {
		t.Fatal("Baseline candidate must be preserved")
	}
}

// TestRestoreReplaceStagedSurvivesRestoresWhenLiveAbsent proves the
// restart rule for the replace race with an absent live tree: the surviving
// staged tree has no persisted physical proof, so recovery preserves the
// recovery slot and the staging with ErrAmbiguous instead of restoring on a
// fresh sample.
func TestRestoreReplaceStagedSurvivesRestoresWhenLiveAbsent(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	if _, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeForTest(s.baselineCandidateDir(2), replacement.dir); err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	if err := renameNoReplaceAt(layout.store, "alpha", layout.recovery, "2"); err != nil {
		t.Fatal(err)
	}

	err = s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore with absent live: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may appear")
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatal("staging and candidate must be preserved")
	}
}

// TestRestoreReplaceConsumedStagedRequiresCandidate proves the pre-rename
// Baseline candidate is required even when the staged tree was consumed: a
// replace whose candidate is missing cannot attribute the live tree to the
// operation and refuses with ErrAmbiguous, preserving every candidate.
func TestRestoreReplaceConsumedStagedRequiresCandidate(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	if err := os.RemoveAll(s.baselineCandidateDir(2)); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("consumed staged without candidate: got %v, want ErrAmbiguous", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); readErr != nil || string(data) != "new content" {
		t.Fatalf("live content must be preserved: %q, %v", data, readErr)
	}
	if data, err := os.ReadFile(filepath.Join(s.recoveryDir(2), "SKILL.md")); err != nil || string(data) != "old content" {
		t.Fatalf("recovery content must be preserved: %q, %v", data, err)
	}
}
