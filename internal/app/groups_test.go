package app

import (
	"context"
	"path/filepath"
	"testing"
)

// importAllSkills imports every Inventory entry of one Local Source and
// returns slug -> Skill id.
func importAllSkills(t *testing.T, a *App, slugs ...string) map[string]int64 {
	t.Helper()
	entries := map[string]string{}
	for _, slug := range slugs {
		entries["skills/"+slug] = slug
	}
	src := addLocalSource(t, a, entries)
	result, err := a.ImportSkills(context.Background(), ImportSkillsInput{SourceID: src.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, item := range result.Items {
		if item.Status != StatusImported {
			t.Fatalf("import item %s: %s", item.RelativeDir, item.Status)
		}
		out[item.Slug] = item.SkillID
	}
	return out
}

func TestCreateAndListGroups(t *testing.T) {
	a := newTestApp(t)
	g, err := a.CreateGroup("engineering")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID == 0 || g.Name != "engineering" {
		t.Fatalf("group: %+v", g)
	}
	// duplicate names conflict
	if _, err := a.CreateGroup("engineering"); err == nil || err.(*Error).Code != CodeConflict {
		t.Fatalf("duplicate group: got %v, want conflict", err)
	}
	// invalid names
	for _, name := range []string{"", "  ", "a/b", ".", ".."} {
		if _, err := a.CreateGroup(name); err == nil || err.(*Error).Code != CodeInvalidArgument {
			t.Fatalf("name %q: got %v, want invalid_argument", name, err)
		}
	}
	items, err := a.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].MemberCount != 0 {
		t.Fatalf("list: %+v", items)
	}
}

// TestCreateGroupTrimsWhitespace pins that surrounding whitespace is
// trimmed before validation and persistence: the stored name is the
// operator-facing value, and a whitespace-only name stays invalid.
func TestCreateGroupTrimsWhitespace(t *testing.T) {
	a := newTestApp(t)
	g, err := a.CreateGroup("  ops  ")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "ops" {
		t.Fatalf("stored name: %q, want %q", g.Name, "ops")
	}
	// the trimmed value is what collides and what resolves
	if _, err := a.CreateGroup("ops"); err == nil || err.(*Error).Code != CodeConflict {
		t.Fatalf("trimmed duplicate: got %v, want conflict", err)
	}
	id, err := a.ResolveGroupArg("ops")
	if err != nil || id != g.ID {
		t.Fatalf("resolve trimmed name: %d, %v", id, err)
	}
	// whitespace-only input is invalid, not stored
	if _, err := a.CreateGroup("   "); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("whitespace-only: got %v", err)
	}
}

func TestGroupMembershipFlow(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta")
	g, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	view, err := a.AddGroupSkills(g.ID, []int64{ids["alpha"], ids["beta"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Members) != 2 {
		t.Fatalf("members: %+v", view.Members)
	}
	// idempotent add
	if _, err := a.AddGroupSkills(g.ID, []int64{ids["alpha"]}); err != nil {
		t.Fatal(err)
	}
	view, err = a.ShowGroup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Members) != 2 || view.Members[0].Slug != "alpha" {
		t.Fatalf("members after idempotent add: %+v", view.Members)
	}
	view, err = a.RemoveGroupSkills(g.ID, []int64{ids["beta"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Members) != 1 || view.Members[0].Slug != "alpha" {
		t.Fatalf("members after remove: %+v", view.Members)
	}
	// unknown Skill and Group
	if _, err := a.AddGroupSkills(g.ID, []int64{999}); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("unknown skill: got %v", err)
	}
	if _, err := a.AddGroupSkills(999, nil); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("unknown group: got %v", err)
	}
	// resolution by name and id agree
	byName, err := a.ResolveGroupArg("eng")
	if err != nil || byName != g.ID {
		t.Fatalf("resolve by name: %d, %v", byName, err)
	}
	if _, err := a.ResolveGroupArg("nope"); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("resolve missing: %v", err)
	}
}

// TestGroupViewShowsTargets pins that Group detail links the Targets that
// assign it.
func TestGroupViewShowsTargets(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	g, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(g.ID, []int64{ids["alpha"]}); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	tv, err := a.RegisterTarget(TargetInput{Path: filepath.Join(target, "skills")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "group", GroupID: g.ID}); err != nil {
		t.Fatal(err)
	}
	view, err := a.ShowGroup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Targets) != 1 || view.Targets[0].ID != tv.ID {
		t.Fatalf("group targets: %+v", view.Targets)
	}
	if view.Targets[0].Name == "" {
		t.Fatal("target name missing")
	}
}
