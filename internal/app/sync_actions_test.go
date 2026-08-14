package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// importOneSkill registers a Local Source and imports the entry at relDir.
func importOneSkill(t *testing.T, a *App, relDir string) (*source.Source, int64) {
	t.Helper()
	src := addLocalSource(t, a, map[string]string{relDir: "demo"})
	res := importSkills(t, a, src.ID, relDir)
	if len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("import items: %+v", res.Items)
	}
	return src, res.Items[0].SkillID
}

// rewriteSourceFile replaces one file below the Source root.
func rewriteSourceFile(t *testing.T, src *source.Source, relDir, file, content string) {
	t.Helper()
	full := filepath.Join(src.Location, filepath.FromSlash(relDir), filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rewriteStoreFile replaces one file below the live Store Skill.
func rewriteStoreFile(t *testing.T, a *App, slug, file, content string) {
	t.Helper()
	full := filepath.Join(a.StorePath, slug, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkSourceNow re-observes one Source and fails the test on error.
func checkSourceNow(t *testing.T, a *App, src *source.Source) *source.Source {
	t.Helper()
	checked, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	return checked
}

// skillDetailOf reads one Skill's committed detail and fails the test on
// error.
func skillDetailOf(t *testing.T, a *App, skillID int64) *state.SkillDetail {
	t.Helper()
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSyncSafeRules locks the decision-05 safe-sync matrix: in_sync is a
// no-op, source_changed is the only automatic update, store_changed and
// conflict are skipped, and missing states block or skip.
func TestSyncSafeRules(t *testing.T) {
	// in_sync → no_op
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	r, err := a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultNoOp || r.Status != sync.StatusInSync {
		t.Fatalf("in_sync sync: %+v", r)
	}

	// source_changed → updated
	oldDigest := findEntry(t, src, "skills/demo").Digest
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream change\n")
	r, err = a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultUpdated || r.Status != sync.StatusInSync || r.BeforeDigest != oldDigest {
		t.Fatalf("source_changed sync: %+v", r)
	}
	if r.AfterDigest == "" || r.AfterDigest == oldDigest {
		t.Fatalf("after digest not advanced: %+v", r)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "notes.md")); err != nil || string(data) != "upstream change\n" {
		t.Fatalf("live content not updated: %q, %v", data, err)
	}
	// The update rotated the previous snapshot with reason sync.
	snap, err := state.GetSnapshot(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != oldDigest || snap.Reason != "sync" {
		t.Fatalf("snapshot after sync: %+v", snap)
	}

	// store_changed → skipped
	rewriteStoreFile(t, a, "demo", "local.txt", "local edit\n")
	r, err = a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultSkipped || r.Status != sync.StatusStoreChanged {
		t.Fatalf("store_changed sync: %+v", r)
	}

	// conflict → skipped
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "another upstream change\n")
	r, err = a.SyncSkill(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultSkipped || r.Status != sync.StatusConflict {
		t.Fatalf("conflict sync: %+v", r)
	}

	// source_missing → blocked
	missing := addLocalSource(t, a, map[string]string{"skills/gone": "ghost"})
	res := importSkills(t, a, missing.ID, "skills/gone")
	goneID := res.Items[0].SkillID
	if err := os.RemoveAll(filepath.Join(missing.Location, "skills", "gone")); err != nil {
		t.Fatal(err)
	}
	r, err = a.SyncSkill(context.Background(), goneID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultBlocked || r.Status != sync.StatusSourceMissing {
		t.Fatalf("source_missing sync: %+v", r)
	}

	// store_missing → skipped (needs explicit accept source)
	repairSrc := addLocalSource(t, a, map[string]string{"skills/repair": "repairme"})
	res = importSkills(t, a, repairSrc.ID, "skills/repair")
	repairID := res.Items[0].SkillID
	if err := os.RemoveAll(filepath.Join(a.StorePath, "repairme")); err != nil {
		t.Fatal(err)
	}
	r, err = a.SyncSkill(context.Background(), repairID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultSkipped || r.Status != sync.StatusStoreMissing {
		t.Fatalf("store_missing sync: %+v", r)
	}

	// The persisted latest outcomes survived the journey.
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != string(sync.StatusConflict) || d.Skill.LastSyncAction != sync.ActionSync || d.Skill.LastSyncResult != sync.ResultSkipped {
		t.Fatalf("persisted sync state: %+v", d.Skill)
	}
}

// TestKeepStoreAdvancesBaseline locks the Keep Store semantics: live
// content untouched, the observed Source becomes the Baseline, and the
// relationship becomes store_changed.
func TestKeepStoreAdvancesBaseline(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	storeDigestBefore := findEntry(t, src, "skills/demo").Digest
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	newSrcDigest := checkSourceNow(t, a, src).Entries[0].Digest

	r, err := a.KeepStore(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultKeptStore || r.Status != sync.StatusStoreChanged {
		t.Fatalf("keep-store: %+v", r)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "demo", "SKILL.md")); err != nil || len(data) == 0 {
		t.Fatalf("store content must be untouched: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "demo", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("store must not receive the Source change")
	}
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.BaselineDigest != newSrcDigest || d.Skill.StoreDigest != storeDigestBefore {
		t.Fatalf("digests after keep-store: baseline %s store %s", d.Skill.BaselineDigest, d.Skill.StoreDigest)
	}
	// The Baseline tree now carries the Source content for future diffs.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "notes.md")); err != nil || string(data) != "upstream\n" {
		t.Fatalf("baseline tree not advanced: %q, %v", data, err)
	}
	// keep-store with the already-accepted baseline is a no-op.
	r, err = a.KeepStore(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != sync.ResultNoOp {
		t.Fatalf("second keep-store: %+v", r)
	}
	// A later Source change becomes a new conflict, per decision 05.
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream 2\n")
	checked, err := a.CheckSkillSync(context.Background(), skillID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("after keep-store + source change: %q", checked.SyncStatus)
	}
}
