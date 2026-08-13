package state

import (
	"database/sql"
	"testing"
	"time"
)

// insertTestTarget registers one minimal custom Target and returns its id.
func insertTestTarget(t *testing.T, db *sql.DB, name, path string) int64 {
	t.Helper()
	now := time.Now().UTC()
	id, err := InsertTarget(db, Target{Name: name, Path: path, Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// insertTestGroup registers one minimal Group and returns its id.
func insertTestGroup(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	now := time.Now().UTC()
	id, err := InsertGroup(db, Group{Name: name, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAssignmentInsertIdempotentAndDelete(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	targetID := insertTestTarget(t, db, "t", "/tmp/t")
	skillID := insertTestSkill(t, db, "alpha")
	groupID := insertTestGroup(t, db, "eng")

	id, created, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "skill", SkillID: skillID, CreatedAt: now})
	if err != nil || !created {
		t.Fatalf("first insert: id %d created %v err %v", id, created, err)
	}
	again, created, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "skill", SkillID: skillID, CreatedAt: now})
	if err != nil || created || again != id {
		t.Fatalf("duplicate insert: id %d created %v err %v, want id %d", again, created, err, id)
	}

	gID, created, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "group", GroupID: groupID, CreatedAt: now})
	if err != nil || !created {
		t.Fatalf("group insert: id %d created %v err %v", gID, created, err)
	}

	details, err := ListTargetAssignmentDetails(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 2 {
		t.Fatalf("details: %+v", details)
	}
	if details[0].Kind != "skill" || details[0].SkillSlug != "alpha" || details[0].SkillName != "Alpha" {
		t.Fatalf("skill detail: %+v", details[0])
	}
	if details[1].Kind != "group" || details[1].GroupName != "eng" {
		t.Fatalf("group detail: %+v", details[1])
	}

	one, err := GetAssignmentByID(db, id)
	if err != nil || one.Kind != "skill" || one.SkillID != skillID {
		t.Fatalf("get by id: %+v, %v", one, err)
	}

	if err := DeleteTargetAssignmentBySubject(db, targetID, "skill", skillID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTargetAssignmentBySubject(db, targetID, "skill", skillID); err != sql.ErrNoRows {
		t.Fatalf("deleting a missing Assignment: got %v, want sql.ErrNoRows", err)
	}
	if err := DeleteAssignment(db, gID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAssignment(db, gID); err != sql.ErrNoRows {
		t.Fatalf("deleting a missing Assignment by id: got %v, want sql.ErrNoRows", err)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestInsertAssignmentConcurrentDuplicates pins the idempotency invariant
// under concurrent duplicates: a batch of simultaneous inserts of the same
// subject must never surface a UNIQUE violation — exactly one call reports
// created, every other call returns the same winning Assignment id.
func TestInsertAssignmentConcurrentDuplicates(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	targetID := insertTestTarget(t, db, "t", "/tmp/t")
	skillID := insertTestSkill(t, db, "alpha")

	const workers = 8
	start := make(chan struct{})
	type outcome struct {
		id      int64
		created bool
		err     error
	}
	results := make(chan outcome, workers)
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			id, created, err := InsertAssignment(db, Assignment{
				TargetID: targetID, Kind: "skill", SkillID: skillID, CreatedAt: now,
			})
			results <- outcome{id: id, created: created, err: err}
		}()
	}
	close(start)

	created := 0
	winning := int64(0)
	var ids []int64
	for i := 0; i < workers; i++ {
		out := <-results
		if out.err != nil {
			t.Fatalf("concurrent insert: %v", out.err)
		}
		ids = append(ids, out.id)
		if out.created {
			created++
			winning = out.id
		}
	}
	if created != 1 {
		t.Fatalf("exactly one insert may create the row, got %d", created)
	}
	// every duplicate must report the winning id
	for _, id := range ids {
		if id != winning {
			t.Fatalf("insert reported id %d, want winning %d", id, winning)
		}
	}
	// the row count stays one
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM assignments WHERE target_id = ? AND kind = 'skill'`,
		targetID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("row count after concurrent duplicates: %d, want 1", n)
	}
}
