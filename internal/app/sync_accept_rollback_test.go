package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// TestAcceptSourceReplaces locks Accept Source: the Store content is
// replaced, the Baseline advances, the previous snapshot records the
// superseded Store content with reason accept_source, and rollback undoes
// it.
func TestAcceptSourceReplaces(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteStoreFile(t, a, "demo", "SKILL.md", "---\nname: demo\ndescription: desc\n---\n# local body\n")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("pre-state: %q", checked.SyncStatus)
	}

	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultAcceptedSource || r.Status != sync.StatusInSync {
		t.Fatalf("accept-source: %+v", r)
	}
	localDigest := r.BeforeDigest // the displaced live content
	if localDigest == "" {
		t.Fatal("accept-source must report the displaced live digest")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); err != nil {
		t.Fatalf("store must receive the Source change: %v", err)
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != localDigest || snap.Reason != "accept_source" {
		t.Fatalf("snapshot after accept: %+v", snap)
	}
	// The snapshot tree physically holds the superseded content.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "previous", itoa(skillID), "SKILL.md")); err != nil ||
		len(data) == 0 || string(data) == "upstream\n" {
		t.Fatalf("previous snapshot content wrong: %q, %v", data, err)
	}
}

// TestAcceptSourceRepairsMissing locks the repair path: an absent Store
// tree is restored by explicit Accept Source without a snapshot rotation.
func TestAcceptSourceRepairsMissing(t *testing.T) {
	a := newTestApp(t)
	_, skillID := importOneSkill(t, a, "skills/demo")
	if err := os.RemoveAll(filepath.Join(a.StorePath, "demo")); err != nil {
		t.Fatal(err)
	}
	r, err := a.AcceptSource(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultAcceptedSource || r.Status != sync.StatusInSync {
		t.Fatalf("repair accept: %+v", r)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "SKILL.md")); err != nil {
		t.Fatalf("store not repaired: %v", err)
	}
	if _, err := state.GetSnapshot(a.db, skillID); err == nil {
		t.Fatal("repair must not rotate a snapshot")
	}
}

// TestRollbackRestoresAndReverses locks the one-step rollback: content
// restored, Binding/relationships untouched, the pre-rollback live tree
// becomes the new snapshot, and a second rollback reverses the first.
func TestRollbackRestoresAndReverses(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	originalDigest := findEntry(t, src, "skills/demo").Digest
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	r, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v", r)
	}
	updatedDigest := r.AfterDigest
	before := skillDetailOf(t, a, skillID)

	rolled, err := a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Result != sync.ResultRolledBack || rolled.AfterDigest != originalDigest {
		t.Fatalf("rollback: %+v", rolled)
	}
	if rolled.Status != sync.StatusStoreChanged {
		t.Fatalf("rollback status: %q", rolled.Status)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("rolled-back content must not carry the upstream file")
	}
	after := skillDetailOf(t, a, skillID)
	if after.Binding.Digest != before.Binding.Digest || after.Binding.SourceCommit != before.Binding.SourceCommit ||
		after.Binding.SourceID != before.Binding.SourceID || after.Binding.RelativeDir != before.Binding.RelativeDir {
		t.Fatalf("rollback must not change the Binding: %+v -> %+v", before.Binding, after.Binding)
	}
	if after.Skill.BaselineDigest != before.Skill.BaselineDigest {
		t.Fatalf("rollback must not change the Baseline: %q -> %q", before.Skill.BaselineDigest, after.Skill.BaselineDigest)
	}
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != updatedDigest || snap.Reason != "rollback" {
		t.Fatalf("snapshot after rollback: %+v", snap)
	}

	// The reverse rollback restores the accepted content.
	reversed, err := a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if reversed.Result != sync.ResultRolledBack || reversed.AfterDigest != updatedDigest {
		t.Fatalf("reverse rollback: %+v", reversed)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("reverse rollback content: %q, %v", data, err)
	}
	// A Skill without a snapshot refuses to roll back.
	fresh := addLocalSource(t, a, map[string]string{"skills/fresh": "fresher"})
	res := importSkills(t, a, fresh.ID, "skills/fresh")
	freshID := res.Items[0].SkillID
	if _, err := a.Rollback(context.Background(), freshID); err == nil {
		t.Fatal("rollback without a snapshot must fail")
	}
}

// TestRollbackBlocksOnLiveChange locks the confirmation re-check: live
// content that changed since the last persisted digest blocks the rollback
// without touching anything.
func TestRollbackBlocksOnLiveChange(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.SyncSkill(context.Background(), skillID); err != nil || r.Result != sync.ResultUpdated {
		t.Fatalf("sync: %+v, %v", r, err)
	}
	rewriteStoreFile(t, a, "demo", "sneaky.txt", "external edit\n")
	r, err := a.Rollback(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultBlocked {
		t.Fatalf("rollback with live change: %+v", r)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "sneaky.txt")); err != nil {
		t.Fatalf("live content must be untouched: %v", err)
	}
}

// TestSyncUnavailableMarksStale locks the availability rule: an
// unreachable Source blocks retrieval, retains the last relationship, and
// marks it stale.
func TestSyncUnavailableMarksStale(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusInSync) || checked.SyncStale {
		t.Fatalf("fresh check: %+v", checked)
	}
	if err := os.RemoveAll(src.Location); err != nil {
		t.Fatal(err)
	}
	r, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultBlocked || !r.Stale {
		t.Fatalf("unavailable sync: %+v", r)
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != string(sync.StatusInSync) || !d.Skill.SyncStale {
		t.Fatalf("relationship must be retained and stale: %q stale=%v", d.Skill.SyncStatus, d.Skill.SyncStale)
	}
}
