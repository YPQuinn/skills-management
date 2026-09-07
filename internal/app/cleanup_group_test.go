package app

import (
	"context"
	"testing"
)

func TestGroupDeleteRequiresUnassign(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	g, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(g.ID, []int64{ids["alpha"]}); err != nil {
		t.Fatal(err)
	}
	tv := registerCustomTarget(t, a)
	if _, err := a.AssignTarget(tv.ID, AssignInput{Kind: "group", GroupID: g.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeleteGroup(context.Background(), g.ID, false); err == nil {
		t.Fatal("assigned Group must be blocked")
	}
	res, err := a.DeleteGroup(context.Background(), g.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unassigned) != 1 || res.Unassigned[0].ID != tv.ID {
		t.Fatalf("unassigned: %+v", res)
	}
	view, err := a.ShowTarget(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Groups) != 0 {
		t.Fatalf("group assignment must be gone: %+v", view.Groups)
	}
}
