package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// TestManualBlockedOutcomePersistsStatus locks the blocked paths of the
// explicit actions: an unavailable Source and a missing or invalid Source
// entry all evaluate and persist the relationship (retaining the previous
// status marked stale when the Source is unreachable) and record a complete
// outcome instead of leaving the item status empty.
func TestManualBlockedOutcomePersistsStatus(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")

	// Unavailable Source: the previous in_sync relationship is retained
	// and marked stale, on the item and in the persisted state.
	if err := os.RemoveAll(src.Location); err != nil {
		t.Fatal(err)
	}
	r, err := a.KeepStore(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultBlocked || r.Status != sync.StatusInSync || !r.Stale {
		t.Fatalf("unavailable keep-store: %+v", r)
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != string(sync.StatusInSync) || !d.Skill.SyncStale ||
		d.Skill.LastSyncAction != sync.ActionKeepStore || d.Skill.LastSyncResult != sync.ResultBlocked {
		t.Fatalf("persisted blocked state: %+v", d.Skill)
	}

	// Missing Source entry: the fresh source_missing relationship is
	// persisted on both the item and the state.
	src2 := addLocalSource(t, a, map[string]string{"skills/other": "Other"})
	res2 := importSkills(t, a, src2.ID, "skills/other")
	if res2.Items[0].Status != StatusImported {
		t.Fatalf("import: %+v", res2.Items)
	}
	otherID := res2.Items[0].SkillID
	if err := os.RemoveAll(filepath.Join(src2.Location, "skills", "other")); err != nil {
		t.Fatal(err)
	}
	r, err = a.AcceptSource(context.Background(), otherID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultBlocked || r.Status != sync.StatusSourceMissing || r.Stale {
		t.Fatalf("missing-entry accept: %+v", r)
	}
	d, err = state.GetSkillDetailByID(a.db, otherID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != string(sync.StatusSourceMissing) || d.Skill.SyncStale {
		t.Fatalf("persisted missing-entry state: %+v", d.Skill)
	}
}

// TestManualNoOpPersistsStatus locks the no-op path: a keep-store whose
// Baseline already carries the observed Source still evaluates and persists
// the relationship, so the item and the persisted state never show an empty
// status.
func TestManualNoOpPersistsStatus(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	if r, err := a.KeepStore(context.Background(), skillID); err != nil || r.Result != sync.ResultKeptStore {
		t.Fatalf("keep-store: %+v, %v", r, err)
	}
	// The Baseline already equals the Source: the second keep-store is a
	// no-op that still reports and persists the evaluated relationship.
	r, err := a.KeepStore(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultNoOp || r.Status != sync.StatusStoreChanged || r.Stale {
		t.Fatalf("no-op keep-store: %+v", r)
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != string(sync.StatusStoreChanged) ||
		d.Skill.LastSyncAction != sync.ActionKeepStore || d.Skill.LastSyncResult != sync.ResultNoOp {
		t.Fatalf("persisted no-op state: %+v", d.Skill)
	}
}
