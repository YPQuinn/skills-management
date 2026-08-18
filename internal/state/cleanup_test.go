package state

import (
	"testing"
	"time"
)

func TestCleanupDeletesReferencedSkill(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	alpha := insertTestSkill(t, db, "alpha")
	eng := insertTestGroup(t, db, "eng")
	if err := AddGroupMembers(db, eng, []int64{alpha}, now); err != nil {
		t.Fatal(err)
	}
	tgt := insertTestTarget(t, db, "t1", "/tmp/t1")
	if _, _, err := InsertAssignment(db, Assignment{TargetID: tgt, Kind: "skill", SkillID: alpha, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertManagedLink(db, ManagedLink{
		TargetID: tgt, SkillID: alpha, LinkPath: "/tmp/t1/alpha",
		RawTarget: "/store/alpha", EstablishedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	links, err := ListManagedLinksBySkill(db, alpha)
	if err != nil || len(links) != 1 || links[0].TargetName != "t1" {
		t.Fatalf("links: %+v, %v", links, err)
	}
	as, err := ListSkillAssignments(db, alpha)
	if err != nil || len(as) != 1 {
		t.Fatalf("assignments: %+v, %v", as, err)
	}

	if err := DeleteSkillAssignments(db, alpha); err != nil {
		t.Fatal(err)
	}
	if err := DeleteSkillGroupMemberships(db, alpha); err != nil {
		t.Fatal(err)
	}
	if err := DeleteManagedLink(db, tgt, alpha); err != nil {
		t.Fatal(err)
	}
	if err := DeleteSkillRow(db, alpha); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSkillDetailByID(db, alpha); err == nil {
		t.Fatal("skill row must be gone")
	}
	if err := DeleteTarget(db, tgt); err != nil {
		t.Fatal(err)
	}
	if err := DeleteGroup(db, eng); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertBindingReplacesAndDetaches(t *testing.T) {
	db := openTestDB(t)
	srcA := testSourceID(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at, available)
		VALUES ('local', '/tmp/other', '', '', 'other', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	srcB, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	alpha := insertTestSkill(t, db, "alpha")
	imported := time.Now().UTC()
	if err := UpsertBinding(db, Binding{SkillID: alpha, SourceID: srcA, RelativeDir: "skills/a", Digest: "d1", ImportedAt: imported}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertBinding(db, Binding{SkillID: alpha, SourceID: srcB, RelativeDir: "skills/b", Digest: "d2", ImportedAt: imported}); err != nil {
		t.Fatal(err)
	}
	d, err := GetSkillDetailByID(db, alpha)
	if err != nil || d.Binding == nil || d.Binding.SourceID != srcB || d.Binding.RelativeDir != "skills/b" {
		t.Fatalf("rebind: %+v, %v", d, err)
	}
	if err := DeleteBinding(db, alpha); err != nil {
		t.Fatal(err)
	}
	d, err = GetSkillDetailByID(db, alpha)
	if err != nil || d.Binding != nil {
		t.Fatalf("detach: %+v, %v", d, err)
	}
	if err := DeleteSource(db, srcA); err != nil {
		t.Fatal(err)
	}
}
