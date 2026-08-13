package state

import (
	"testing"
	"time"
)

func TestGetSkillImpact(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	alpha := insertTestSkill(t, db, "alpha")
	eng := insertTestGroup(t, db, "eng")
	ops := insertTestGroup(t, db, "ops")
	if err := AddGroupMembers(db, eng, []int64{alpha}, now); err != nil {
		t.Fatal(err)
	}
	if err := AddGroupMembers(db, ops, []int64{alpha}, now); err != nil {
		t.Fatal(err)
	}
	direct := insertTestTarget(t, db, "direct", "/tmp/direct")
	viaGroup := insertTestTarget(t, db, "via", "/tmp/via")
	both := insertTestTarget(t, db, "both", "/tmp/both")
	if _, _, err := InsertAssignment(db, Assignment{TargetID: direct, Kind: "skill", SkillID: alpha, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InsertAssignment(db, Assignment{TargetID: viaGroup, Kind: "group", GroupID: eng, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InsertAssignment(db, Assignment{TargetID: both, Kind: "skill", SkillID: alpha, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InsertAssignment(db, Assignment{TargetID: both, Kind: "group", GroupID: ops, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// an unrelated Target is not affected
	insertTestTarget(t, db, "other", "/tmp/other")

	imp, err := GetSkillImpact(db, alpha)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Groups) != 2 || imp.Groups[0].Name != "eng" || imp.Groups[1].Name != "ops" {
		t.Fatalf("groups: %+v", imp.Groups)
	}
	if len(imp.Targets) != 3 {
		t.Fatalf("targets: %+v", imp.Targets)
	}
	byName := map[string]TargetImpact{}
	for _, tg := range imp.Targets {
		byName[tg.Name] = tg
	}
	if !byName["direct"].Direct || len(byName["direct"].Groups) != 0 {
		t.Fatalf("direct target: %+v", byName["direct"])
	}
	if byName["via"].Direct || len(byName["via"].Groups) != 1 || byName["via"].Groups[0].Name != "eng" {
		t.Fatalf("via target: %+v", byName["via"])
	}
	if !byName["both"].Direct || len(byName["both"].Groups) != 1 || byName["both"].Groups[0].Name != "ops" {
		t.Fatalf("both target: %+v", byName["both"])
	}
	if _, ok := byName["other"]; ok {
		t.Fatal("unrelated Target must not be affected")
	}
}
