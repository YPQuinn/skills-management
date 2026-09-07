package app

import (
	"path/filepath"
	"testing"
)

// registerCustomTarget registers one custom Target under a fresh temp dir
// with a unique name.
func registerCustomTarget(t *testing.T, a *App) *TargetView {
	t.Helper()
	dir := t.TempDir()
	view, err := a.RegisterTarget(TargetInput{
		Path: filepath.Join(dir, "skills"), Name: "custom-" + filepath.Base(dir),
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestAssignUnassignSkillsAndGroups(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta")
	g, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(g.ID, []int64{ids["alpha"], ids["beta"]}); err != nil {
		t.Fatal(err)
	}
	tv := registerCustomTarget(t, a)

	as, err := a.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: ids["alpha"]})
	if err != nil {
		t.Fatal(err)
	}
	if as.Skill == nil || as.Skill.Slug != "alpha" {
		t.Fatalf("skill assignment: %+v", as)
	}
	// idempotent re-assign reuses the same Assignment
	again, err := a.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: ids["alpha"]})
	if err != nil || again.ID != as.ID {
		t.Fatalf("re-assign: %+v, %v", again, err)
	}
	gAs, err := a.AssignTarget(tv.ID, AssignInput{Kind: "group", GroupID: g.ID})
	if err != nil {
		t.Fatal(err)
	}
	if gAs.Group == nil || gAs.Group.Name != "eng" {
		t.Fatalf("group assignment: %+v", gAs)
	}

	view, err := a.ShowTarget(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.DirectSkills) != 1 || len(view.Groups) != 1 {
		t.Fatalf("assignments: %+v / %+v", view.DirectSkills, view.Groups)
	}
	// unassign by subject refreshes the view
	view, err = a.UnassignTarget(tv.ID, "group", g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Groups) != 0 {
		t.Fatalf("after unassign: %+v", view.Groups)
	}
	// unassigning a missing Assignment is not_found
	if _, err := a.UnassignTarget(tv.ID, "group", g.ID); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("unassign missing: got %v", err)
	}
	// unassign by Assignment id
	view, err = a.UnassignTargetByID(tv.ID, as.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.DirectSkills) != 0 {
		t.Fatalf("after id unassign: %+v", view.DirectSkills)
	}
	// an Assignment of another Target cannot be removed through this one
	other := registerCustomTarget(t, a)
	foreign, err := a.AssignTarget(other.ID, AssignInput{Kind: "skill", SkillID: ids["beta"]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UnassignTargetByID(tv.ID, foreign.ID); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("foreign assignment: got %v", err)
	}

	// validation and existence errors
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "skill"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("missing subject: got %v", err)
	}
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "wat", SkillID: 1}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("bad kind: got %v", err)
	}
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: 999}); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("missing skill: got %v", err)
	}
	if _, err := a.AssignTarget(999, AssignInput{Kind: "skill", SkillID: ids["alpha"]}); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("missing target: got %v", err)
	}
}

// TestDesiredSetExpansionWithReasons pins the desired-set semantics: the
// union of direct and group selections, deduplicated by Skill, with every
// contributing Assignment as a reason.

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
