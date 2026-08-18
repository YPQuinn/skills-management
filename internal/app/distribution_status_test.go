package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/distribution"
)

// writeStoreSkill writes one valid Store Skill tree for repair scenarios.
func writeStoreSkill(t *testing.T, store, slug string) {
	t.Helper()
	dir := filepath.Join(store, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := "---\nname: " + slug + "\ndescription: test\n---\n# " + slug + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDistributionBrokenLinkAndStoreMissing proves a proven link whose
// destination is gone blocks as broken_link, and a desired Skill whose
// Store tree is missing blocks instead of being created broken.
func TestDistributionBrokenLinkAndStoreMissing(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo", "ghost")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	assignSkill(t, a, tv.ID, ids["ghost"])
	res, err := a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil || res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("initial distribute: %+v, %v", res, err)
	}

	if err := os.RemoveAll(filepath.Join(a.StorePath, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(a.StorePath, "ghost")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tv.Path, "ghost")); err != nil {
		t.Fatal(err)
	}
	st, err := a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range st.Items {
		if item.Slug == "demo" && item.Observed != "broken_link" {
			t.Fatalf("demo: %+v", item)
		}
	}
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultBlocked {
		t.Fatalf("outcome: %+v", res)
	}
	for _, item := range res.Items {
		if item.Result != distribution.OutcomeBlockedBroken {
			t.Fatalf("item %s: %+v", item.Slug, item)
		}
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "ghost")); !os.IsNotExist(err) {
		t.Fatal("a link to a missing Store Skill must not be created")
	}

	writeStoreSkill(t, a.StorePath, "demo")
	writeStoreSkill(t, a.StorePath, "ghost")
	st, err = a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Items[0].Observed != "linked" || st.Items[1].Observed != "missing" {
		t.Fatalf("repaired: %+v", st.Items)
	}
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != distribution.OutcomeNoOp || res.Items[1].Result != distribution.OutcomeCreated {
		t.Fatalf("repaired distribute: %+v", res.Items)
	}
}

// TestOwnershipLostOnReplacement proves a replaced previously-managed path
// is left untouched, its invalid claim is discarded, and the result
// reports ownership_lost.
func TestOwnershipLostOnReplacement(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	if res, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil || res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("initial distribute: %+v, %v", res, err)
	}
	if _, err := a.UnassignTarget(tv.ID, "skill", ids["demo"]); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(tv.Path, "demo")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != distribution.OutcomeOwnershipLost {
		t.Fatalf("items: %+v", res.Items)
	}
	if data, err := os.ReadFile(link); err != nil || string(data) != "user content" {
		t.Fatalf("replacement touched: %q, %v", data, err)
	}
	view, err := a.ShowTarget(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Distribution == nil || len(view.Distribution.Items) != 0 {
		t.Fatalf("relinquished relation must vanish: %+v", view.Distribution)
	}
}

// TestRedirectedTargetBlocksMutation proves a symlink introduced after
// registration redirects the Target and receives no mutations.
func TestRedirectedTargetBlocksMutation(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])

	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, tv.Path); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tv.Path)

	st, err := a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "redirected" || !st.Stale || st.InspectionError == "" {
		t.Fatalf("redirected status: %+v", st)
	}
	res, err := a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultFailed || !strings.Contains(res.Error, "resolves to") {
		t.Fatalf("redirected distribute: %+v", res)
	}
	if _, err := os.Lstat(filepath.Join(elsewhere, "demo")); !os.IsNotExist(err) {
		t.Fatal("the redirected location must not receive links")
	}
}
