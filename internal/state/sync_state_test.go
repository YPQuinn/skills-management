package state

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/skillstore"
)

func newSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestSyncStatusAndOutcomeRoundtrip locks the persisted comparison state
// and latest-outcome columns.
func TestSyncStatusAndOutcomeRoundtrip(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	now := time.Now().UTC()
	if err := UpdateSyncStatus(db, skillID, "source_changed", true, now); err != nil {
		t.Fatal(err)
	}
	out := SyncOutcome{
		Action: "sync", Result: "updated", StartedAt: now, CompletedAt: now,
		BeforeDigest: "d1", AfterDigest: "d2", Revision: "abc", Error: "",
	}
	if err := UpdateSyncOutcome(db, skillID, out); err != nil {
		t.Fatal(err)
	}
	d, err := GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != "source_changed" || !d.Skill.SyncStale || d.Skill.SyncCheckedAt == nil {
		t.Fatalf("sync status: %+v", d.Skill)
	}
	if d.Skill.LastSyncAction != "sync" || d.Skill.LastSyncResult != "updated" ||
		d.Skill.LastSyncBeforeDigest != "d1" || d.Skill.LastSyncAfterDigest != "d2" ||
		d.Skill.LastSyncRevision != "abc" || d.Skill.LastSyncCompletedAt == nil {
		t.Fatalf("sync outcome: %+v", d.Skill)
	}
}

// TestCommitSyncReplaceReason locks the snapshot reason distinction
// between an explicit Replace, a sync update, and an Accept Source.
func TestCommitSyncReplaceReason(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	s := Skill{ID: skillID, Slug: "demo", Name: "Demo", Description: "d",
		StoreDigest: "new", BaselineDigest: "new", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	b := Binding{SkillID: skillID, SourceID: 1, RelativeDir: "skills/demo",
		Digest: "new", SourceCommit: "c1", ImportedAt: time.Now().UTC()}
	op := skillstore.Operation{SkillID: skillID, Slug: "demo", Kind: skillstore.KindReplace,
		OldDigest: "old", NewDigest: "new"}
	op, err := InsertOperation(db, op)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitSyncReplace(db, s, b, op.ID, "accept_source"); err != nil {
		t.Fatal(err)
	}
	snap, err := GetSnapshot(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != "old" || snap.Reason != "accept_source" {
		t.Fatalf("snapshot: %+v", snap)
	}
	// markAcceptedTx recorded the fresh in_sync relationship.
	d, err := GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.SyncStatus != "in_sync" || d.Skill.SyncStale {
		t.Fatalf("accepted relationship: %+v", d.Skill)
	}
}

// TestCommitSyncReplaceAcceptsLiveDigest locks the conflict acceptance:
// the journal's OldDigest may describe locally edited live content that
// differs from the persisted Store digest.
func TestCommitSyncReplaceAcceptsLiveDigest(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	s := Skill{ID: skillID, Slug: "demo", Name: "Demo", Description: "d",
		StoreDigest: "new", BaselineDigest: "new", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	b := Binding{SkillID: skillID, SourceID: 1, RelativeDir: "skills/demo",
		Digest: "new", SourceCommit: "c1", ImportedAt: time.Now().UTC()}
	op, err := InsertOperation(db, skillstore.Operation{SkillID: skillID, Slug: "demo",
		Kind: skillstore.KindReplace, OldDigest: "local-edit", NewDigest: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitSyncReplace(db, s, b, op.ID, "accept_source"); err != nil {
		t.Fatal(err)
	}
	snap, err := GetSnapshot(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != "local-edit" {
		t.Fatalf("snapshot must carry the displaced live digest: %+v", snap)
	}
}

// TestCommitRollbackPreservesIdentity locks the rollback commit: Store
// digest becomes the snapshot digest, Baseline and Binding stay untouched,
// and the snapshot rotates with reason rollback.
func TestCommitRollbackPreservesIdentity(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	op, err := InsertOperation(db, skillstore.Operation{SkillID: skillID, Slug: "demo",
		Kind: skillstore.KindReplace, OldDigest: "cur", NewDigest: "snap",
		BaselineMode: skillstore.BaselineKeep})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRollback(db, Skill{ID: skillID, Slug: "demo",
		StoreDigest: "snap", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, op.ID); err != nil {
		t.Fatal(err)
	}
	d, err := GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.StoreDigest != "snap" || d.Skill.BaselineDigest != "base" {
		t.Fatalf("rollback digests: %+v", d.Skill)
	}
	if d.Binding == nil || d.Binding.Digest != "base" || d.Binding.SourceCommit != "c0" {
		t.Fatalf("rollback binding: %+v", d.Binding)
	}
	snap, err := GetSnapshot(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != "cur" || snap.Reason != "rollback" || snap.SourceCommit != "c0" {
		t.Fatalf("rollback snapshot: %+v", snap)
	}
}

// TestCommitRollbackRefusesAdvancingOperation locks the guard: a rollback
// commit never blesses an advancing (baseline-replacing) journal row.
func TestCommitRollbackRefusesAdvancingOperation(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	op, err := InsertOperation(db, skillstore.Operation{SkillID: skillID, Slug: "demo",
		Kind: skillstore.KindReplace, OldDigest: "cur", NewDigest: "snap"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRollback(db, Skill{ID: skillID, Slug: "demo",
		StoreDigest: "snap", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, op.ID); err == nil {
		t.Fatal("rollback commit must refuse an advancing operation")
	}
}

// TestCommitRollbackWithoutBinding locks the Binding-independent rollback
// commit: a Skill whose Source Binding was removed still rolls back, and
// the rotated snapshot records empty Source evidence.
func TestCommitRollbackWithoutBinding(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	if _, err := db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		t.Fatal(err)
	}
	op, err := InsertOperation(db, skillstore.Operation{SkillID: skillID, Slug: "demo",
		Kind: skillstore.KindReplace, OldDigest: "cur", NewDigest: "snap",
		BaselineMode: skillstore.BaselineKeep})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRollback(db, Skill{ID: skillID, Slug: "demo",
		StoreDigest: "snap", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, op.ID); err != nil {
		t.Fatal(err)
	}
	d, err := GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.StoreDigest != "snap" || d.Skill.BaselineDigest != "base" {
		t.Fatalf("rollback digests: %+v", d.Skill)
	}
	if d.Binding != nil {
		t.Fatalf("the Binding must stay absent: %+v", d.Binding)
	}
	snap, err := GetSnapshot(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Digest != "cur" || snap.Reason != "rollback" || snap.SourceCommit != "" {
		t.Fatalf("unbound rollback snapshot must carry empty Source evidence: %+v", snap)
	}
}

// TestCommitBaselineAdvanceCAS locks the Baseline-only refresh commit: the
// Baseline digest and the Binding advance atomically with the operation's
// phase, and a stale CAS on the recorded old Baseline digest fails without
// touching the Binding or the pending operation.
func TestCommitBaselineAdvanceCAS(t *testing.T) {
	db := newSyncTestDB(t)
	skillID := insertSkillRow(t, db, "demo")
	op, err := InsertOperation(db, skillstore.Operation{
		SkillID: skillID, Slug: "demo", Kind: skillstore.KindBaseline,
		OldDigest: "base", NewDigest: "src2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitBaselineAdvance(db, op, skillID, "base", "src2", "c2"); err != nil {
		t.Fatal(err)
	}
	d, err := GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.BaselineDigest != "src2" || d.Binding.Digest != "src2" || d.Binding.SourceCommit != "c2" {
		t.Fatalf("baseline commit: %+v %+v", d.Skill, d.Binding)
	}
	if ops, err := ListOpenOperations(db); err != nil || len(ops) != 1 || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the operation must transition to committed for its finalization: %+v, %v", ops, err)
	}
	// A stale CAS on the old Baseline digest fails atomically.
	op2, err := InsertOperation(db, skillstore.Operation{
		SkillID: skillID, Slug: "demo", Kind: skillstore.KindBaseline,
		OldDigest: "base", NewDigest: "src3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitBaselineAdvance(db, op2, skillID, "base", "src3", "c3"); err == nil {
		t.Fatal("stale Baseline CAS must fail")
	}
	d, err = GetSkillDetailByID(db, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Skill.BaselineDigest != "src2" || d.Binding.Digest != "src2" || d.Binding.SourceCommit != "c2" {
		t.Fatalf("stale CAS must not partially apply: %+v %+v", d.Skill, d.Binding)
	}
	if ops, err := ListOpenOperations(db); err != nil || len(ops) != 2 || ops[1].Phase != skillstore.PhasePending {
		t.Fatalf("the failed operation must stay pending: %+v, %v", ops, err)
	}
}

// TestListSkillsBySource locks the batch lookup.
func TestListSkillsBySource(t *testing.T) {
	db := newSyncTestDB(t)
	_ = insertSkillRow(t, db, "alpha")
	beta := insertSkillRow(t, db, "beta")
	// alpha is bound to Source 1, beta to Source 2 (insertSkillRow binds
	// every Skill to Source 1); register Source 2 and rebind beta.
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO sources (id, kind, location, ref, subpath, name, created_at, updated_at)
		VALUES (2, 'local', '/tmp/src2', '', '', 'src2', ?, ?)`, timeToSQL(&now), timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE source_bindings SET source_id = 2 WHERE skill_id = ?`, beta); err != nil {
		t.Fatal(err)
	}
	rows, err := ListSkillsBySource(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Skill.Slug != "alpha" {
		t.Fatalf("bindings of Source 1: %+v", rows)
	}
	rows, err = ListSkillsBySource(db, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Skill.Slug != "beta" {
		t.Fatalf("bindings of Source 2: %+v", rows)
	}
}

// insertSkillRow persists one Skill bound to Source 1 with distinct
// digests and returns its id.
func insertSkillRow(t *testing.T, db *sql.DB, slug string) int64 {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO sources (id, kind, location, ref, subpath, name, created_at, updated_at)
		VALUES (1, 'local', '/tmp/src1', '', '', 'src1', ?, ?) ON CONFLICT(id) DO NOTHING`, timeToSQL(&now), timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	id, err := InsertSkillAndBinding(db, Skill{
		Slug: slug, Name: slug, Description: "desc",
		StoreDigest: "cur", BaselineDigest: "base", CreatedAt: now, UpdatedAt: now,
	}, Binding{SourceID: 1, RelativeDir: "skills/" + slug, Digest: "base",
		SourceCommit: "c0", ImportedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
