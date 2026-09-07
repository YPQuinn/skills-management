package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/distribution"
)

func TestTargetDeleteRemovesManagedLinksKeepsContainer(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])
	if _, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil {
		t.Fatal(err)
	}
	res, err := a.DeleteTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Result != distribution.OutcomeRemoved {
		t.Fatalf("target delete: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("link: %v", err)
	}
	if _, err := os.Lstat(tv.Path); err != nil {
		t.Fatalf("container: %v", err)
	}
	if _, err := a.ShowTarget(tv.ID); err == nil {
		t.Fatal("Target registration must be gone")
	}
	if _, err := a.ShowSkill(ids["alpha"]); err != nil {
		t.Fatalf("Skill must remain: %v", err)
	}
}
