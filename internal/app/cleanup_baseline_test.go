package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

func TestClearBaselineFreshAppFinishesAfterCommitCrash(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	d := skillDetailOf(t, a, id)
	op := isolateBaselineClearAndStop(t, a, id, "alpha", d.Skill.BaselineDigest)
	if err := state.CommitBaselineClear(a.db, op, id, d.Skill.BaselineDigest, nil, string(sync.StatusUnbound)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(a.store.BaselineDir(id)); !os.IsNotExist(err) {
		t.Fatal("Baseline must stay isolated until committed recovery drains it")
	}

	fresh := reopenApp(t, a)
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("committed Baseline clear must drain and clear: %+v", ops)
	}
	got := skillDetailOf(t, fresh, id)
	if got.Binding != nil || got.Skill.BaselineDigest != "" || got.Skill.SyncStatus != string(sync.StatusUnbound) {
		t.Fatalf("committed recovery must keep Detach: %+v", got)
	}
	if _, err := os.Lstat(fresh.store.BaselineDir(id)); !os.IsNotExist(err) {
		t.Fatalf("parked Baseline must be drained: %v", err)
	}
}

func TestDetachSkillCommitFailureRestoresBaseline(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	id := ids["alpha"]
	d := skillDetailOf(t, a, id)
	oldBaseline := d.Skill.BaselineDigest
	oldSource := d.Binding.SourceID
	a.commitBaselineClear = func(skillstore.Operation, int64, string, *state.Binding, string) error {
		return os.ErrInvalid
	}
	if _, err := a.DetachSkill(context.Background(), id); err == nil {
		t.Fatal("want Detach failure when Baseline-clear commit fails")
	}
	got := skillDetailOf(t, a, id)
	if got.Binding == nil || got.Binding.SourceID != oldSource || got.Skill.BaselineDigest != oldBaseline {
		t.Fatalf("failed Detach must keep Binding and digest: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(a.store.BaselineDir(id), "SKILL.md")); err != nil {
		t.Fatalf("failed Detach must restore the Baseline tree: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("failed Detach must not leave a journal: %+v", ops)
	}
}

func TestSourceDeleteDetachCommitFailureKeepsSource(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	srcs, err := a.ListSources()
	if err != nil || len(srcs) != 1 {
		t.Fatalf("sources: %+v, %v", srcs, err)
	}
	a.commitBaselineClear = func(skillstore.Operation, int64, string, *state.Binding, string) error {
		return os.ErrInvalid
	}
	if _, err := a.DeleteSource(context.Background(), srcs[0].ID, true); err == nil {
		t.Fatal("want Source delete failure when detach commit fails")
	}
	if _, err := a.ShowSource(srcs[0].ID); err != nil {
		t.Fatalf("failed detach must keep the Source: %v", err)
	}
	got := skillDetailOf(t, a, ids["alpha"])
	if got.Binding == nil || got.Binding.SourceID != srcs[0].ID {
		t.Fatalf("failed Source delete must keep the Binding: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(a.store.BaselineDir(ids["alpha"]), "SKILL.md")); err != nil {
		t.Fatalf("failed Source delete must restore the Baseline: %v", err)
	}
}
