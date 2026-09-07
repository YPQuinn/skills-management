package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// TestRollbackCancelBeforeCommit locks the durable rollback recovery: a
// cancellation exactly between Install and the SQLite commit unwinds the
// installed live tree through the terminal restore protocol, keeps the
// previous snapshot and the Baseline, and leaves no open operation.
func TestRollbackCancelBeforeCommit(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.SyncSkill(context.Background(), skillID); err != nil || r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	preDigest := skillDetailOf(t, a, skillID).Skill.StoreDigest
	preBaseline := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	preSnapshot, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}

	// Cancel exactly at the pre-commit seam: Rollback must unwind the
	// installed live tree through the terminal restore protocol.
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	rolled, err := a.Rollback(ctx, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result != sync.ResultFailed || rolled.ErrorCode != CodeCancelled {
		t.Fatalf("cancelled rollback: %+v", rolled)
	}
	// The live content is the pre-rollback content again (restore).
	detail := skillDetailOf(t, a, skillID)
	if detail.Skill.StoreDigest != preDigest {
		t.Fatalf("store digest after restore: %q, want %q", detail.Skill.StoreDigest, preDigest)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("live content after restore: %q, %v", data, err)
	}
	if detail.Skill.BaselineDigest != preBaseline {
		t.Fatalf("baseline mutated by the failed rollback")
	}
	snapAfter, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snapAfter.Digest != preSnapshot.Digest || snapAfter.Reason != preSnapshot.Reason {
		t.Fatalf("snapshot changed by the failed rollback: %+v -> %+v", preSnapshot, snapAfter)
	}
	// No open operations remain: the terminal restore completed.
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("open operations after restore: %+v", ops)
	}
}

// TestRollbackKeepsBaselineAndSnapshot locks the finalize of a successful
// keep-baseline replace: the Baseline tree is untouched while the live and
// previous trees are exactly the journaled digests.
func TestRollbackKeepsBaselineAndSnapshot(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.SyncSkill(context.Background(), skillID); err != nil || r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	// The sync update advanced the Baseline; the rollback must retain that
	// accepted Source content untouched.
	baseDigest := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	if r, err := a.Rollback(context.Background(), skillID); err != nil || r.Result != sync.ResultRolledBack {
		t.Fatalf("rollback: %+v, %v", r, err)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != baseDigest {
		t.Fatalf("baseline digest changed: %q -> %q", baseDigest, d.Skill.BaselineDigest)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("baseline tree mutated: %q, %v", data, err)
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason != "rollback" || snap.Digest == "" {
		t.Fatalf("snapshot: %+v", snap)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("open operations after rollback: %+v", ops)
	}
}

// TestConvergedCheckAdvancesBaseline locks the independent-convergence
// rule: when the Source and the Store meet on content the Baseline does not
// carry, a check records in_sync and advances the Baseline — together with
// the Binding's accepted digest and revision — to the freshly sampled live
// content, never to the stale persisted Store digest.
func TestConvergedCheckAdvancesBaseline(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	// Store diverges first; the persisted Store digest goes stale.
	rewriteStoreFile(t, a, "demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# locally edited\n")
	liveDigest, missing, invalid := a.storeTreeState(context.Background(), "demo")
	if missing || invalid || liveDigest == "" {
		t.Fatalf("live store state: %q missing=%v invalid=%v", liveDigest, missing, invalid)
	}
	// The Source then moves to exactly the same content.
	if err := os.RemoveAll(filepath.Join(src.Location, "skills", "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src.Location, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src.Location, "skills", "demo", "SKILL.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusInSync) || checked.SyncStale {
		t.Fatalf("converged check: %+v", checked)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != liveDigest {
		t.Fatalf("baseline not advanced to the converged live digest: baseline %q want %q", d.Skill.BaselineDigest, liveDigest)
	}
	// The Binding records the converged content as its accepted identity.
	if d.Binding.Digest != liveDigest || d.Binding.SourceCommit != checked.Binding.SourceCommit {
		t.Fatalf("binding not advanced: %+v", d.Binding)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "SKILL.md")); err != nil ||
		len(data) == 0 {
		t.Fatalf("baseline tree not advanced: %v", err)
	}
}
