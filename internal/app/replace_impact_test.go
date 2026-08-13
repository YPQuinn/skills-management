package app

import (
	"context"
	"testing"
)

func TestReplaceImpactPreview(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	result, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	alpha := result.Items[0].SkillID

	eng, err := a.CreateGroup("eng")
	if err != nil {
		t.Fatal(err)
	}
	ops, err := a.CreateGroup("ops")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(eng.ID, []int64{alpha}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddGroupSkills(ops.ID, []int64{alpha}); err != nil {
		t.Fatal(err)
	}
	direct := registerCustomTarget(t, a)
	if _, err := a.AssignTarget(direct.ID, AssignInput{Kind: "skill", SkillID: alpha}); err != nil {
		t.Fatal(err)
	}
	via := registerCustomTarget(t, a)
	if _, err := a.AssignTarget(via.ID, AssignInput{Kind: "group", GroupID: eng.ID}); err != nil {
		t.Fatal(err)
	}
	both := registerCustomTarget(t, a)
	if _, err := a.AssignTarget(both.ID, AssignInput{Kind: "skill", SkillID: alpha}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AssignTarget(both.ID, AssignInput{Kind: "group", GroupID: ops.ID}); err != nil {
		t.Fatal(err)
	}
	registerCustomTarget(t, a) // unrelated

	// a second Source exposing a same-named entry claims the same slug:
	// the skipped-conflict preview carries the impact
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	conflict, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := conflict.Items[0]
	if item.Status != StatusSkippedConflict || item.Impact == nil {
		t.Fatalf("conflict item: %+v", item)
	}
	if len(item.Impact.Groups) != 2 {
		t.Fatalf("impact groups: %+v", item.Impact.Groups)
	}
	byName := map[string]TargetImpact{}
	for _, tg := range item.Impact.Targets {
		byName[tg.Name] = tg
	}
	if len(byName) != 3 {
		t.Fatalf("impact targets: %+v", byName)
	}
	if !byName[direct.Name].Direct || len(byName[direct.Name].Groups) != 0 {
		t.Fatalf("direct impact: %+v", byName[direct.Name])
	}
	if byName[via.Name].Direct || len(byName[via.Name].Groups) != 1 || byName[via.Name].Groups[0].Name != "eng" {
		t.Fatalf("via impact: %+v", byName[via.Name])
	}
	if !byName[both.Name].Direct || len(byName[both.Name].Groups) != 1 || byName[both.Name].Groups[0].Name != "ops" {
		t.Fatalf("both impact: %+v", byName[both.Name])
	}

	// the replaced outcome carries the same impact
	replaced, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha", Replace: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Items[0].Status != StatusReplaced || replaced.Items[0].Impact == nil ||
		len(replaced.Items[0].Impact.Targets) != 3 {
		t.Fatalf("replaced item: %+v", replaced.Items[0])
	}
}
