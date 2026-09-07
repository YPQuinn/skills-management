package state

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// v6FS returns a migration set containing only 001-006, the schema the
// previous build left on disk.
func v6FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{
		"001_initial.sql", "002_sources.sql", "003_source_observation.sql",
		"004_unique_source_name.sql", "005_skills.sql", "006_terminal_receipts.sql",
	} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration007GroupsTargets is the upgrade path: a database created by
// the previous build (schema version 6) holds Sources, Skills, Bindings,
// and operations. Opening it with the current executable applies migration
// 007, which adds the Groups, Targets, membership, and Assignment tables
// while preserving every existing row.
func TestMigration007GroupsTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v6FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 6 {
		t.Fatalf("legacy schema: got %d, want 6", got)
	}
	srcID := testSourceID(t, db)
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 13 {
		t.Fatalf("migrated schema: got %d, want 13", got)
	}
	// The upgraded schema accepts the new entities and keeps the old rows.
	now := time.Now().UTC()
	groupID, err := InsertGroup(db, Group{Name: "engineering", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := AddGroupMembers(db, groupID, []int64{skillID}, now); err != nil {
		t.Fatal(err)
	}
	targetID, err := InsertTarget(db, Target{
		Name: "custom-1", Path: "/tmp/target", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	asID, created, err := InsertAssignment(db, Assignment{
		TargetID: targetID, Kind: "group", GroupID: groupID, CreatedAt: now,
	})
	if err != nil || !created || asID == 0 {
		t.Fatalf("assignment insert: id %d created %v err %v", asID, created, err)
	}
	if _, err := GetSkillDetailByID(db, skillID); err != nil {
		t.Fatalf("pre-migration Skill lost: %v", err)
	}
}

// TestAssignmentConstraints pins the structural safety invariants: one
// subject of either kind per Target, the kind/subject CHECK, and the
// default blocking of referenced Skill and Group deletion.
func TestAssignmentConstraints(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	srcID := testSourceID(t, db)
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := InsertTarget(db, Target{Name: "t", Path: "/tmp/t", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = InsertAssignment(db, Assignment{TargetID: targetID, Kind: "skill", SkillID: skillID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	// the idempotent insert returns the existing row rather than duplicating
	again, againCreated, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "skill", SkillID: skillID, CreatedAt: now})
	if err != nil || againCreated {
		t.Fatalf("duplicate insert must reuse the existing row: id %d created %v err %v", again, againCreated, err)
	}
	// the unique index itself still rejects a raw duplicate insert
	if _, err := db.Exec(`INSERT INTO assignments (target_id, kind, skill_id, group_id, created_at)
		VALUES (?, 'skill', ?, NULL, ?)`, targetID, skillID, timeToSQL(&now)); err == nil {
		t.Fatal("duplicate skill Assignment must violate the unique index")
	}
	// a kind/subject mismatch violates the CHECK
	if _, err := db.Exec(`INSERT INTO assignments (target_id, kind, skill_id, group_id, created_at)
		VALUES (?, 'skill', NULL, ?, ?)`, targetID, groupID, timeToSQL(&now)); err == nil {
		t.Fatal("skill Assignment with a group subject must be rejected")
	}
	// deleting a referenced Skill is blocked by default
	if _, err := db.Exec(`DELETE FROM skills WHERE id = ?`, skillID); err == nil {
		t.Fatal("deleting an assigned Skill must be blocked")
	}
	// deleting a referenced Group is blocked too
	if _, _, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "group", GroupID: groupID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM groups WHERE id = ?`, groupID); err == nil {
		t.Fatal("deleting an assigned Group must be blocked")
	}
	// removing the Assignments first unlocks deletion
	if _, err := db.Exec(`DELETE FROM assignments WHERE target_id = ?`, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM skills WHERE id = ?`, skillID); err != nil {
		t.Fatalf("deleting an unassigned Skill must succeed: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM groups WHERE id = ?`, groupID); err != nil {
		t.Fatalf("deleting an unassigned Group must succeed: %v", err)
	}
}

// TestGroupTargetNameUniqueness pins the operator-facing name and path
// identity constraints.
func TestGroupTargetNameUniqueness(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	if _, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now}); err == nil ||
		!strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("duplicate Group name must be a unique violation, got %v", err)
	}
	if _, err := InsertTarget(db, Target{Name: "a", Path: "/tmp/one", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertTarget(db, Target{Name: "b", Path: "/tmp/one", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now}); err == nil ||
		!strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("duplicate Target path must be a unique violation, got %v", err)
	}
	if _, err := InsertTarget(db, Target{Name: "a", Path: "/tmp/two", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now}); err == nil ||
		!strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("duplicate Target name must be a unique violation, got %v", err)
	}
}
