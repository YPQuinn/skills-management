package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// TestAcceptSourceFailureKeepsBaseline locks the atomic Accept Source
// semantics: a failure between the intent and the SQLite commit leaves the
// persisted Baseline and its tree exactly as they were, the live Store
// restored, and the three-party relationship intact — the next automatic
// synchronization must still classify the local edit as a conflict and
// never auto-overwrite it. A retried Accept Source then commits with
// Baseline equal to the Source.
func TestAcceptSourceFailureKeepsBaseline(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	preBaseline := skillDetailOf(t, a, skillID).Skill.BaselineDigest
	rewriteStoreFile(t, a, "demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# local body\n")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if checked, err := a.CheckSkillSync(context.Background(), skillID); err != nil || checked.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("pre-state: %+v, %v", checked, err)
	}

	// Fail exactly between Install and the SQLite commit: the terminal
	// restore must unwind the installed live tree.
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	r, err := a.AcceptSource(ctx, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeCancelled {
		t.Fatalf("cancelled accept: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != preBaseline {
		t.Fatalf("failed accept mutated the persisted Baseline: %q -> %q", preBaseline, d.Skill.BaselineDigest)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "SKILL.md")); err != nil ||
		string(data) != "---\nname: demo\ndescription: desc\n---\n# demo\n" {
		t.Fatalf("failed accept mutated the Baseline tree: %q, %v", data, err)
	}
	// The live content is restored to the local edit.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md")); err != nil || len(data) == 0 {
		t.Fatalf("live content not restored: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("live content must not carry the Source change: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the failed accept must leave no open operation: %+v", ops)
	}

	// The relationship stays a conflict: an automatic sync must skip, never
	// overwrite the local edit (a pre-aligned Baseline would classify the
	// same facts as source_changed and auto-update).
	a.commitHook = nil
	synced, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if synced.Result != sync.ResultSkipped || synced.Status != sync.StatusConflict {
		t.Fatalf("sync after failed accept must skip the conflict: %+v", synced)
	}

	// The retry commits atomically: Baseline becomes the Source content.
	r, err = a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultAcceptedSource || r.Status != sync.StatusInSync {
		t.Fatalf("retried accept: %+v", r)
	}
	d = skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != r.AfterDigest || d.Skill.StoreDigest != r.AfterDigest || d.Binding.Digest != r.AfterDigest {
		t.Fatalf("committed accept digests: %+v", d)
	}
	// The displaced local content became the previous snapshot.
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != r.BeforeDigest || snap.Reason != "accept_source" {
		t.Fatalf("snapshot after accept: %+v", snap)
	}
}

// TestAcceptSourceBlockedFinalizeConverges locks the post-commit failure
// window: when the terminal cleanup of a committed Accept Source fails, the
// committed Baseline equals the Source and the next Accept Source converges
// through recovery instead of re-mutating the Baseline.
func TestAcceptSourceBlockedFinalizeConverges(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")

	a.deleteOperation = func(int64) error { return os.ErrInvalid }
	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultFailed || r.ErrorCode != CodeRecovery {
		t.Fatalf("blocked accept: %+v", r)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != r.AfterDigest {
		t.Fatalf("committed accept must carry the accepted Baseline: %+v", d)
	}
	if ops := openOperations(t, a); len(ops) != 1 {
		t.Fatalf("the unresolved operation must stay open: %+v", ops)
	}

	// The next attempt recovers the finalized operation first and then
	// reports the converged no-op.
	a.deleteOperation = nil
	r, err = a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultNoOp || r.Status != sync.StatusInSync {
		t.Fatalf("converged accept: %+v", r)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the open operation: %+v", ops)
	}
}

// TestAcceptSourceRepairsPlainFile locks the store_invalid repair for live
// content that is not a Skill tree at all: a plain file is detected as
// store_invalid, Accept Source displaces it through the journal, the
// digests and the relationship are persisted, and no snapshot exists for
// the unreadable content.
func TestAcceptSourceRepairsPlainFile(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	if err := os.RemoveAll(filepath.Join(a.StorePath, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.StorePath, "demo"), []byte("not a skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusStoreInvalid) {
		t.Fatalf("plain file must classify as store_invalid: %+v", checked)
	}
	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultAcceptedSource || r.Status != sync.StatusInSync {
		t.Fatalf("repair accept: %+v", r)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md")); err != nil || len(data) == 0 {
		t.Fatalf("store not repaired: %v", err)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.StoreDigest != r.AfterDigest || d.Skill.BaselineDigest != r.AfterDigest || d.Binding.Digest != r.AfterDigest {
		t.Fatalf("repair digests not persisted: %+v", d)
	}
	if _, err := state.GetSnapshot(a.db, skillID); err == nil {
		t.Fatal("unreadable content must not rotate a snapshot")
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("repair must leave no open operation: %+v", ops)
	}
	// The check afterwards stays in_sync, not invalid.
	checked, err = a.CheckSkillSync(context.Background(), skillID)
	if err != nil || checked.SyncStatus != string(sync.StatusInSync) {
		t.Fatalf("post-repair check: %+v, %v", checked, err)
	}
	_ = src
}

// TestAcceptSourceRepairsInvalidSkill locks the store_invalid repair for a
// Store tree whose SKILL.md marker is not a legal Skill: it is detected by
// the shared Skill validator, Accept Source snapshots the invalid content
// as the previous snapshot, and the committed Baseline equals the Source.
func TestAcceptSourceRepairsInvalidSkill(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	_, skillID := importOneSkill(t, a, "skills/demo")
	rewriteStoreFile(t, a, "demo", "SKILL.md", "just a note, no frontmatter\n")
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusStoreInvalid) {
		t.Fatalf("invalid SKILL.md must classify as store_invalid: %+v", checked)
	}
	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultAcceptedSource || r.Status != sync.StatusInSync {
		t.Fatalf("repair accept: %+v", r)
	}
	if r.BeforeDigest == "" {
		t.Fatal("the displaced invalid tree must be reported")
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != r.BeforeDigest || snap.Reason != "accept_source" {
		t.Fatalf("snapshot after repair: %+v", snap)
	}
	d := skillDetailOf(t, a, skillID)
	if d.Skill.BaselineDigest != r.AfterDigest || d.Binding.Digest != r.AfterDigest {
		t.Fatalf("repair digests not persisted: %+v", d)
	}
	// The repaired Store carries a legal Skill marker again.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md")); err != nil ||
		len(data) == 0 {
		t.Fatalf("store not repaired: %v", err)
	}
}
