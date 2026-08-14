package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestBaselineFinalizeResumesAfterSwap locks the crash window of the
// Baseline-only refresh: a re-preparation after the semantic swap (the
// crash-before-receipt window) returns the same effective receipt, the
// receipt-bound cleanup then converges, the Baseline tree carries the new
// content, and the untouched live tree keeps its identity.
func TestBaselineFinalizeResumesAfterSwap(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old baseline\n"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	liveBefore := liveDigestOf(t, s, "alpha")

	fresh := buildMaterialized(t, map[string]string{"SKILL.md": "new baseline\n"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindBaseline,
		OldDigest: old.digest, NewDigest: fresh.digest}
	if _, err := s.Stage(context.Background(), op.ID, fresh.dir, fresh.digest, true); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil || string(data) != "new baseline\n" {
		t.Fatalf("baseline after prepare: %q, %v", data, err)
	}
	// The crash-before-receipt resume re-prepares the same terminal state
	// and returns the same effective receipt.
	again, err := s.PrepareFinalize(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Bytes()) != string(receipt.Bytes()) {
		t.Fatalf("re-prepared receipt differs: %q vs %q", again.Bytes(), receipt.Bytes())
	}
	op.Phase = PhaseFinalized
	if err := s.CleanupTerminal(context.Background(), op, again); err != nil {
		t.Fatal(err)
	}
	if liveDigestOf(t, s, "alpha") != liveBefore {
		t.Fatal("the live tree must never change during a Baseline refresh")
	}
	if data, err := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil || string(data) != "new baseline\n" {
		t.Fatalf("baseline after cleanup: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(2)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("operation evidence must be cleared: %v", err)
	}
}

// TestBaselineRestorePreservesSurvivingStaging locks the shared restore
// contract: a pending Baseline refresh whose staging survives a restart is
// never attributed to the operation and never drained; the Baseline and the
// live tree stay exactly as they were and every candidate is preserved.
func TestBaselineRestorePreservesSurvivingStaging(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old baseline\n"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	fresh := buildMaterialized(t, map[string]string{"SKILL.md": "new baseline\n"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindBaseline,
		OldDigest: old.digest, NewDigest: fresh.digest}
	if _, err := s.Stage(context.Background(), op.ID, fresh.dir, fresh.digest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareRestore(context.Background(), op); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restore with surviving staging: got %v, want ErrAmbiguous", err)
	}
	if data, err := os.ReadFile(filepath.Join(s.BaselineDir(7), "SKILL.md")); err != nil || string(data) != "old baseline\n" {
		t.Fatalf("baseline mutated: %q, %v", data, err)
	}
	if _, err := os.Lstat(s.opDir(2)); err != nil {
		t.Fatalf("staging must be preserved: %v", err)
	}
}

// TestBaselineRestoreNeverStartedIsClearable locks the clearable state: a
// pending Baseline refresh without any operation evidence returns the Empty
// receipt, so the caller certifies the absence through the abort protocol
// without touching content.
func TestBaselineRestoreNeverStartedIsClearable(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 9, SkillID: 7, Slug: "alpha", Kind: KindBaseline,
		OldDigest: "old", NewDigest: "new"}
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Empty() {
		t.Fatalf("never-started restore: %+v", receipt)
	}
}
