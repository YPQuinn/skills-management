package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

func TestSkillDeleteBlockedThenCleansManagedLinks(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])
	if _, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeleteSkill(context.Background(), ids["alpha"], false); err == nil {
		t.Fatal("referenced Skill must be blocked")
	}
	preview, err := a.PreviewDeleteSkill(ids["alpha"])
	if err != nil || !preview.Referenced || len(preview.Links) != 1 {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	res, err := a.DeleteSkill(context.Background(), ids["alpha"], true)
	if err != nil {
		t.Fatal(err)
	}
	if res.RemovedAssignments != 1 || len(res.Links) != 1 || res.Links[0].Result != distribution.OutcomeRemoved {
		t.Fatalf("delete: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("managed link must be removed: %v", err)
	}
	if _, err := os.Lstat(tv.Path); err != nil {
		t.Fatalf("Target container must remain: %v", err)
	}
	if _, err := a.ShowSkill(ids["alpha"]); err == nil {
		t.Fatal("Skill must be gone")
	}
}

func TestSkillDeleteLeavesOwnershipLostPath(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])
	if _, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tv.Path, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tv.Path, "alpha"), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := a.DeleteSkill(context.Background(), ids["alpha"], true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Result != distribution.OutcomeOwnershipLost {
		t.Fatalf("ownership lost: %+v", res)
	}
	data, err := os.ReadFile(filepath.Join(tv.Path, "alpha"))
	if err != nil || string(data) != "foreign" {
		t.Fatalf("unmanaged replacement must remain: %q, %v", data, err)
	}
}

func TestSkillDeleteRecoversPendingCreateIntent(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	t1 := registerCustomTarget(t, a)
	assignSkill(t, a, t1.ID, ids["alpha"])
	if _, err := a.DistributeTarget(context.Background(), t1.ID, false); err != nil {
		t.Fatal(err)
	}
	t2 := registerCustomTarget(t, a)
	assignSkill(t, a, t2.ID, ids["alpha"])
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(t2.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(root, "alpha")
	proof, err := createFixtureLink(t, t2.Path, "alpha", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: t2.ID, SkillID: ids["alpha"], Action: "create",
		LinkPath: filepath.Join(t2.Path, "alpha"), RawTarget: raw,
		LinkDev: proof.Dev, LinkIno: proof.Ino, LinkMtime: proof.Mtime,
		Phase: state.LinkPhasePlanned, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := a.DeleteSkill(context.Background(), ids["alpha"], true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) < 2 {
		t.Fatalf("both Targets must be cleaned: %+v", res.Links)
	}
	if _, err := os.Lstat(filepath.Join(t1.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("T1 link: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(t2.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("T2 recovered link must be removed: %v", err)
	}
	if intents, err := state.ListOpenLinkIntents(a.db); err != nil || len(intents) != 0 {
		t.Fatalf("intents: %+v, %v", intents, err)
	}
	if _, err := a.ShowSkill(ids["alpha"]); err == nil {
		t.Fatal("Skill must be gone")
	}
}

func TestDeleteSkillRestoresStoreWhenRemoveFailsAfterLiveMoved(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterRemoveSkillLiveMoved {
			return
		}
		rec := filepath.Join(a.StorePath, ".skillctl", "recovery")
		ents, err := os.ReadDir(rec)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if e.IsDir() {
				if err := os.MkdirAll(filepath.Join(rec, e.Name(), "base"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
	if _, err := a.DeleteSkill(context.Background(), id, true); err == nil {
		t.Fatal("want delete failure after live was isolated")
	}
	if _, err := a.ShowSkill(id); err != nil {
		t.Fatalf("cleanup failure must retain the Skill row: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "name: alpha") {
		t.Fatalf("live Store Skill must be restored: %q, %v", data, err)
	}
}

func TestDeleteSkillBlocksConcurrentDirectAssignment(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	tv := registerCustomTarget(t, a)
	other := secondApp(t, a)

	var assignErr error
	a.afterSkillStoreRemoved = func() {
		_, assignErr = other.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: ids["alpha"]})
	}
	if _, err := a.DeleteSkill(context.Background(), ids["alpha"], true); err != nil {
		t.Fatal(err)
	}
	if !isCode(assignErr, CodeLocked) {
		t.Fatalf("concurrent assign in the delete window: %v", assignErr)
	}
	if _, err := a.ShowSkill(ids["alpha"]); err == nil {
		t.Fatal("Skill must be gone")
	}
	if as, err := state.ListSkillAssignments(a.db, ids["alpha"]); err != nil || len(as) != 0 {
		t.Fatalf("no Assignment may land after Store removal: %+v, %v", as, err)
	}
	if _, err := other.AssignTarget(tv.ID, AssignInput{Kind: "skill", SkillID: ids["alpha"]}); !isCode(err, CodeNotFound) {
		t.Fatalf("assign after delete: %v", err)
	}
}
