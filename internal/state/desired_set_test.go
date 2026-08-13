package state

import (
	"testing"
	"time"
)

func TestExpandDesiredSetUnionAndReasons(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	targetID := insertTestTarget(t, db, "t", "/tmp/t")
	alpha := insertTestSkill(t, db, "alpha")
	beta := insertTestSkill(t, db, "beta")
	gamma := insertTestSkill(t, db, "gamma")
	eng := insertTestGroup(t, db, "eng")
	ops := insertTestGroup(t, db, "ops")

	// eng = {alpha, beta}; ops = {beta, gamma}
	if err := AddGroupMembers(db, eng, []int64{alpha, beta}, now); err != nil {
		t.Fatal(err)
	}
	if err := AddGroupMembers(db, ops, []int64{beta, gamma}, now); err != nil {
		t.Fatal(err)
	}

	// target: alpha directly, both groups assigned
	directID, _, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "skill", SkillID: alpha, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	engID, _, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "group", GroupID: eng, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	opsID, _, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "group", GroupID: ops, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	desired, err := ExpandDesiredSet(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	// union {alpha, beta, gamma}, slug order
	if len(desired) != 3 || desired[0].Slug != "alpha" || desired[1].Slug != "beta" || desired[2].Slug != "gamma" {
		t.Fatalf("desired slugs: %+v", desired)
	}
	// alpha: direct + via eng
	if len(desired[0].Reasons) != 2 {
		t.Fatalf("alpha reasons: %+v", desired[0].Reasons)
	}
	var alphaKinds []string
	for _, r := range desired[0].Reasons {
		alphaKinds = append(alphaKinds, r.Kind)
	}
	if !containsString(alphaKinds, "skill") || !containsString(alphaKinds, "group") {
		t.Fatalf("alpha reason kinds: %v", alphaKinds)
	}
	// beta: via both groups, with names
	if len(desired[1].Reasons) != 2 {
		t.Fatalf("beta reasons: %+v", desired[1].Reasons)
	}
	var betaGroups []string
	for _, r := range desired[1].Reasons {
		if r.Kind != "group" || (r.AssignmentID != engID && r.AssignmentID != opsID) {
			t.Fatalf("beta reason: %+v", r)
		}
		betaGroups = append(betaGroups, r.GroupName)
	}
	if !containsString(betaGroups, "eng") || !containsString(betaGroups, "ops") {
		t.Fatalf("beta group names: %v", betaGroups)
	}
	if len(desired[2].Reasons) != 1 || desired[2].Reasons[0].AssignmentID != opsID {
		t.Fatalf("gamma reasons: %+v", desired[2].Reasons)
	}
	_ = directID
}

// TestExpandDesiredSetTracksMembership pins that the desired set is derived
// from current membership: removing a member drops it, removing the group
// Assignment drops the whole group contribution.
func TestExpandDesiredSetTracksMembership(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	targetID := insertTestTarget(t, db, "t", "/tmp/t")
	alpha := insertTestSkill(t, db, "alpha")
	beta := insertTestSkill(t, db, "beta")
	eng := insertTestGroup(t, db, "eng")
	if err := AddGroupMembers(db, eng, []int64{alpha, beta}, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InsertAssignment(db, Assignment{TargetID: targetID, Kind: "group", GroupID: eng, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGroupMembers(db, eng, []int64{beta}); err != nil {
		t.Fatal(err)
	}
	desired, err := ExpandDesiredSet(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || desired[0].Slug != "alpha" {
		t.Fatalf("desired after membership change: %+v", desired)
	}
	if err := DeleteTargetAssignmentBySubject(db, targetID, "group", eng); err != nil {
		t.Fatal(err)
	}
	desired, err = ExpandDesiredSet(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 0 {
		t.Fatalf("desired after unassign: %+v", desired)
	}
}
