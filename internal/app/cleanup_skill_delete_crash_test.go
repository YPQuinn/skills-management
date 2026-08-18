package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

func isolateSkillDeleteAndStop(t *testing.T, a *App, skillID int64, slug string) skillstore.Operation {
	t.Helper()
	live, _, _ := a.storeTreeState(context.Background(), slug)
	if live == "" {
		live = removeDigestAbsent
	}
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: skillID, Slug: slug, Kind: skillstore.KindRemove,
		OldDigest: live, NewDigest: live,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.IsolateRemove(op); err != nil {
		t.Fatal(err)
	}
	return op
}

func TestDeleteSkillFreshAppRestoresAfterLiveIsolateCrash(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	isolateSkillDeleteAndStop(t, a, id, "alpha")
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("live must be isolated before the crash")
	}
	if _, err := a.ShowSkill(id); err != nil {
		t.Fatalf("DB row must remain while the journal is pending: %v", err)
	}

	fresh := reopenApp(t, a)
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("pending remove must restore and clear: %+v", ops)
	}
	if _, err := fresh.ShowSkill(id); err != nil {
		t.Fatalf("Skill row must survive pending recovery: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(fresh.StorePath, "alpha", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "name: alpha") {
		t.Fatalf("live must be restored so the DB does not point at a missing tree: %q, %v", data, err)
	}
}

func TestDeleteSkillRowFailureRestoresStore(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	a.commitSkillDelete = func(skillstore.Operation, int64) error { return os.ErrInvalid }
	if _, err := a.DeleteSkill(context.Background(), id, true); err == nil {
		t.Fatal("want delete failure when the Skill row commit fails")
	}
	if _, err := a.ShowSkill(id); err != nil {
		t.Fatalf("failed row delete must keep the Skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("failed row delete must restore live: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("failed commit must not leave a journal: %+v", ops)
	}
}

func TestDeleteSkillFreshAppFinishesAfterRowCommitCrash(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	op := isolateSkillDeleteAndStop(t, a, id, "alpha")
	if err := state.CommitSkillDelete(a.db, op, id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShowSkill(id); err == nil {
		t.Fatal("committed remove must have deleted the Skill row")
	}

	fresh := reopenApp(t, a)
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("committed remove must drain and clear: %+v", ops)
	}
	if _, err := fresh.ShowSkill(id); err == nil {
		t.Fatal("committed recovery must keep the Skill deleted")
	}
	if _, err := os.Lstat(filepath.Join(fresh.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("parked live must be drained: %v", err)
	}
}
