package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/sync"
)

func TestSourceDeleteRequiresDetachThenKeepsSkills(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	src, err := a.ListSources()
	if err != nil || len(src) != 1 {
		t.Fatalf("sources: %+v, %v", src, err)
	}
	if _, err := a.DeleteSource(context.Background(), src[0].ID, false); err == nil {
		t.Fatal("bound Source must be blocked")
	} else if ae, ok := err.(*Error); !ok || ae.Code != CodeConflict {
		t.Fatalf("blocked: %v", err)
	}
	preview, err := a.PreviewDeleteSource(src[0].ID)
	if err != nil || !preview.RequiresDetach || len(preview.BoundSkills) != 1 {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	res, err := a.DeleteSource(context.Background(), src[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Detached) != 1 || res.Detached[0].Slug != "alpha" {
		t.Fatalf("detached: %+v", res)
	}
	sk, err := a.ShowSkill(ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	if sk.Binding != nil || sk.SyncStatus != string(sync.StatusUnbound) {
		t.Fatalf("skill after detach: %+v", sk)
	}
	if _, err := os.Stat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Store Skill must remain: %v", err)
	}
}

func TestDeleteSourceDoesNotDetachReboundSkill(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	srcs, err := a.ListSources()
	if err != nil || len(srcs) != 1 {
		t.Fatalf("sources: %+v, %v", srcs, err)
	}
	srcA := srcs[0].ID
	srcB := addLocalSource(t, a, map[string]string{"skills/alpha": "alpha"})
	if _, err := a.RebindSkill(context.Background(), ids["alpha"], srcB.ID, "skills/alpha", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DeleteSource(context.Background(), srcA, true); err != nil {
		t.Fatal(err)
	}
	sk, err := a.ShowSkill(ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	if sk.Binding == nil || sk.Binding.SourceID != srcB.ID {
		t.Fatalf("rebound Skill must stay bound to Source B: %+v", sk.Binding)
	}
}

func TestDeleteSourceMissing(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	if _, err := a.DeleteSource(context.Background(), 99, false); err == nil {
		t.Fatal("want not found")
	} else {
		var ae *Error
		if !errors.As(err, &ae) || ae.Code != CodeNotFound {
			t.Fatalf("got %v", err)
		}
	}
}
