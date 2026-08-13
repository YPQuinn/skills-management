package app

import (
	"testing"
)

func TestDesiredSetExpansionWithReasons(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta", "gamma")
	eng, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	ops, err := a.CreateGroup("ops")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(eng.ID, []int64{ids["alpha"], ids["beta"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(ops.ID, []int64{ids["beta"], ids["gamma"]}); err != nil {
		t.Fatal(err)
	}
	tv := registerCustomTarget(t, a)
	direct, err := a.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: ids["alpha"]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "group", GroupID: eng.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "group", GroupID: ops.ID}); err != nil {
		t.Fatal(err)
	}

	desired, err := a.TargetDesiredSet(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 3 || desired[0].Slug != "alpha" || desired[1].Slug != "beta" || desired[2].Slug != "gamma" {
		t.Fatalf("desired: %+v", desired)
	}
	// alpha: direct + via eng
	if len(desired[0].Reasons) != 2 {
		t.Fatalf("alpha reasons: %+v", desired[0].Reasons)
	}
	var alphaKinds []string
	for _, r := range desired[0].Reasons {
		alphaKinds = append(alphaKinds, r.Kind)
		if r.Kind == "skill" && r.AssignmentID != direct.ID {
			t.Fatalf("alpha direct reason: %+v", r)
		}
		if r.Kind == "group" && r.GroupName != "eng" {
			t.Fatalf("alpha group reason: %+v", r)
		}
	}
	if !contains(alphaKinds, "skill") || !contains(alphaKinds, "group") {
		t.Fatalf("alpha kinds: %v", alphaKinds)
	}
	// beta: via both groups
	if len(desired[1].Reasons) != 2 {
		t.Fatalf("beta reasons: %+v", desired[1].Reasons)
	}

	// membership changes update the desired set immediately
	if _, err := a.RemoveGroupSkills(eng.ID, []int64{ids["beta"]}); err != nil {
		t.Fatal(err)
	}
	desired, err = a.TargetDesiredSet(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 3 || len(desired[1].Reasons) != 1 {
		t.Fatalf("desired after membership change: %+v", desired)
	}
	// removing the group Assignment drops its contribution entirely
	if _, err := a.UnassignTarget(tv.ID, "group", ops.ID); err != nil {
		t.Fatal(err)
	}
	desired, err = a.TargetDesiredSet(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	// eng now holds only alpha (beta was removed above), and ops is gone
	if len(desired) != 1 || desired[0].Slug != "alpha" || len(desired[0].Reasons) != 2 {
		t.Fatalf("desired after unassign: %+v", desired)
	}
	if _, err := a.TargetDesiredSet(999); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("missing target desired set: got %v", err)
	}
}

// TestReplaceImpactPreview pins the deferred ticket-12 preview: replacing
// a Skill reports the Groups containing it and the Targets whose desired
// set includes it, directly or through assigned Groups.
