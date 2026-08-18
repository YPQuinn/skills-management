package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

func TestDetachAndRebindSkill(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	sk, err := a.DetachSkill(context.Background(), ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	if sk.Binding != nil || sk.SyncStatus != string(sync.StatusUnbound) {
		t.Fatalf("detached: %+v", sk)
	}

	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	identical, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if !identical.Identical || identical.Skill.Binding == nil || identical.Skill.SyncStatus != string(sync.StatusInSync) {
		t.Fatalf("identical rebind: %+v", identical)
	}

	srcC := addLocalSource(t, a, map[string]string{"skills/other": "other"})
	if err := os.WriteFile(filepath.Join(srcC.Location, "skills", "other", "SKILL.md"),
		[]byte("---\nname: other\ndescription: changed\n---\n# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CheckSource(context.Background(), srcC.ID); err != nil {
		t.Fatal(err)
	}
	conflict, err := a.RebindSkill(context.Background(), ids["alpha"], srcC.ID, "skills/other", "")
	if err != nil {
		t.Fatal(err)
	}
	if conflict.Identical || conflict.Skill.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("different rebind: %+v", conflict)
	}
	if _, err := os.Stat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("rebind must not overwrite Store: %v", err)
	}
}

func TestRebindUnavailableSource(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	if err := os.RemoveAll(srcB.Location); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", ""); err == nil {
		t.Fatal("unavailable Source must block Rebind")
	} else {
		var ae *Error
		if !errors.As(err, &ae) || ae.Code != CodeSourceUnavailable {
			t.Fatalf("got %v", err)
		}
	}
}

func TestIdenticalRebindEstablishesBaseline(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	if _, err := a.DetachSkill(context.Background(), ids["alpha"]); err != nil {
		t.Fatal(err)
	}
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	res, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Identical || res.Skill.SyncStatus != string(sync.StatusInSync) {
		t.Fatalf("identical: %+v", res)
	}
	if res.Skill.BaselineDigest == "" || res.Skill.BaselineDigest != res.Skill.StoreDigest {
		t.Fatalf("baseline digest: store=%s baseline=%s", res.Skill.StoreDigest, res.Skill.BaselineDigest)
	}
	if _, err := os.Stat(filepath.Join(a.store.BaselineDir(ids["alpha"]), "SKILL.md")); err != nil {
		t.Fatalf("Baseline tree: %v", err)
	}
}

func TestRebindIdenticalUsesLiveDigest(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	if err := os.WriteFile(filepath.Join(a.StorePath, "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: local edit\n---\n# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	res, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical || res.Skill.SyncStatus != string(sync.StatusConflict) {
		t.Fatalf("stale store_digest must not count as identical: %+v", res)
	}
}

func TestRebindFinalizeFailureKeepsCommittedJournal(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	if _, err := a.DetachSkill(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	a.receiptPersist = func(skillstore.Operation) error { return os.ErrInvalid }
	if _, err := a.RebindSkill(context.Background(), id, srcB.ID, "skills/alpha", ""); err == nil {
		t.Fatal("receipt failure must fail identical Rebind")
	} else if errorCodeOf(err) != CodeRecovery {
		t.Fatalf("got %v", err)
	}
	sk, err := a.ShowSkill(id)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Binding == nil || sk.Binding.SourceID != srcB.ID {
		t.Fatalf("committed Rebind must keep the new Binding: %+v", sk)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].SkillID != id || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("committed Baseline journal must remain: %+v", ops)
	}
	d := skillDetailOf(t, a, id)
	if d.Skill.BaselineDigest == "" {
		t.Fatalf("committed Baseline digest must stay advanced: %+v", d)
	}
	a.receiptPersist = nil
	checked, err := a.CheckSkillSync(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if checked.SyncStatus != string(sync.StatusInSync) {
		t.Fatalf("recovery must converge: %+v", checked)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the journal: %+v", ops)
	}
}

func TestRebindBaselineVerifyFailureRestoresBinding(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	if _, err := a.DetachSkill(context.Background(), ids["alpha"]); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(a.store.BaselineDir(ids["alpha"]), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.store.BaselineDir(ids["alpha"]), "SKILL.md"),
		[]byte("---\nname: foreign\ndescription: x\n---\n# foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	if _, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", ""); err == nil {
		t.Fatal("foreign Baseline must fail identical Rebind")
	}
	sk, err := a.ShowSkill(ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	if sk.Binding != nil || sk.SyncStatus != string(sync.StatusUnbound) {
		t.Fatalf("failed Rebind must keep the unbound Skill: %+v", sk)
	}
}

func isolateBaselineClearAndStop(t *testing.T, a *App, skillID int64, slug, oldBaseline string) skillstore.Operation {
	t.Helper()
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: skillID, Slug: slug, Kind: skillstore.KindBaseline,
		OldDigest: oldBaseline, NewDigest: baselineClearDigest,
		BaselineMode: skillstore.BaselineClear, BaselineDigest: oldBaseline,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.IsolateBaseline(op); err != nil {
		t.Fatal(err)
	}
	return op
}

func TestRebindClearBaselineFreshAppRestoresAfterIsolateCrash(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	d := skillDetailOf(t, a, id)
	oldBinding := d.Binding.Binding
	oldBaseline := d.Skill.BaselineDigest
	isolateBaselineClearAndStop(t, a, id, "alpha", oldBaseline)
	if _, err := os.Lstat(a.store.BaselineDir(id)); !os.IsNotExist(err) {
		t.Fatal("Baseline must be isolated before the crash")
	}

	fresh := reopenApp(t, a)
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("pending Baseline clear must restore and clear: %+v", ops)
	}
	got := skillDetailOf(t, fresh, id)
	if got.Binding == nil || got.Binding.SourceID != oldBinding.SourceID || got.Skill.BaselineDigest != oldBaseline {
		t.Fatalf("pending recovery must keep Binding and digest: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(fresh.store.BaselineDir(id), "SKILL.md")); err != nil {
		t.Fatalf("Baseline tree must be restored: %v", err)
	}
}

func TestRebindClearBaselineCommitFailureRestoresTree(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	d := skillDetailOf(t, a, id)
	oldBaseline := d.Skill.BaselineDigest
	srcC := addLocalSource(t, a, map[string]string{"skills/other": "other"})
	if err := os.WriteFile(filepath.Join(srcC.Location, "skills", "other", "SKILL.md"),
		[]byte("---\nname: other\ndescription: changed\n---\n# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CheckSource(context.Background(), srcC.ID); err != nil {
		t.Fatal(err)
	}
	a.commitBaselineClear = func(skillstore.Operation, int64, string, *state.Binding, string) error {
		return os.ErrInvalid
	}
	if _, err := a.RebindSkill(context.Background(), id, srcC.ID, "skills/other", ""); err == nil {
		t.Fatal("want Rebind failure when Baseline-clear commit fails")
	}
	got := skillDetailOf(t, a, id)
	if got.Binding == nil || got.Binding.SourceID == srcC.ID || got.Skill.BaselineDigest != oldBaseline {
		t.Fatalf("failed clear commit must keep the old Binding and digest: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(a.store.BaselineDir(id), "SKILL.md")); err != nil {
		t.Fatalf("failed clear commit must restore the Baseline tree: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("failed commit must not leave a journal: %+v", ops)
	}
}
