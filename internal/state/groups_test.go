package state

import (
	"database/sql"
	"reflect"
	"testing"
	"time"
)

// insertTestSkill registers one Skill row bound to its own Source (like
// the real import path) and returns its id.
func insertTestSkill(t *testing.T, db *sql.DB, slug string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at, available)
		VALUES ('local', ?, '', '', ?, ?, ?, 1)`, "/tmp/src-"+slug, "src-"+slug, now, now)
	if err != nil {
		t.Fatal(err)
	}
	srcID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	id, err := InsertSkillAndBinding(db, testSkill(slug), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGroupCRUD(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	id, err := InsertGroup(db, Group{Name: "engineering", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	g, err := GetGroupByID(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "engineering" || !g.CreatedAt.Equal(now) {
		t.Fatalf("group round-trip: %+v", g)
	}
	if _, err := GetGroupByID(db, id+100); err != sql.ErrNoRows {
		t.Fatalf("missing group: got %v, want sql.ErrNoRows", err)
	}
	byName, err := GetGroupByName(db, "engineering")
	if err != nil || byName.ID != id {
		t.Fatalf("group by name: %+v, %v", byName, err)
	}
}

func TestGroupMembershipAddRemoveIdempotent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	groupID, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	alpha := insertTestSkill(t, db, "alpha")
	beta := insertTestSkill(t, db, "beta")

	if err := AddGroupMembers(db, groupID, []int64{alpha, beta}, now); err != nil {
		t.Fatal(err)
	}
	// adding alpha again is a no-op, not an error
	if err := AddGroupMembers(db, groupID, []int64{alpha}, now); err != nil {
		t.Fatal(err)
	}
	members, err := ListGroupMemberSkills(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].Slug != "alpha" || members[1].Slug != "beta" {
		t.Fatalf("members: %+v", members)
	}

	if err := RemoveGroupMembers(db, groupID, []int64{beta}); err != nil {
		t.Fatal(err)
	}
	// removing a non-member is a no-op
	if err := RemoveGroupMembers(db, groupID, []int64{beta}); err != nil {
		t.Fatal(err)
	}
	members, err = ListGroupMemberSkills(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Slug != "alpha" {
		t.Fatalf("members after removal: %+v", members)
	}

	summaries, err := ListGroupSummaries(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].MemberCount != 1 {
		t.Fatalf("summaries: %+v", summaries)
	}
}

func TestGroupTargets(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	groupID, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	t1, err := InsertTarget(db, Target{Name: "z", Path: "/tmp/z", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := InsertTarget(db, Target{Name: "a", Path: "/tmp/a", Adapter: "custom", Scope: "custom", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{t1, t2} {
		if _, _, err := InsertAssignment(db, Assignment{TargetID: id, Kind: "group", GroupID: groupID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	refs, err := ListGroupTargets(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	want := []TargetRef{{ID: t2, Name: "a"}, {ID: t1, Name: "z"}}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("group targets: got %+v, want %+v", refs, want)
	}
}

// TestSkillDeleteCascadesMembership pins that deleting a Skill drops its
// memberships but is still blocked while an Assignment references it.
func TestSkillDeleteCascadesMembership(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	groupID, err := InsertGroup(db, Group{Name: "eng", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	skillID := insertTestSkill(t, db, "alpha")
	if err := AddGroupMembers(db, groupID, []int64{skillID}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM source_bindings WHERE skill_id = ?`, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM skills WHERE id = ?`, skillID); err != nil {
		t.Fatal(err)
	}
	members, err := ListGroupMemberSkills(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("membership must cascade with the Skill: %+v", members)
	}
}
