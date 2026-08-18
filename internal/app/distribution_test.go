package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/distribution"
)

// assignSkill assigns one Skill directly to a Target.
func assignSkill(t *testing.T, a *App, targetID, skillID int64) {
	t.Helper()
	if _, err := a.AssignTarget(targetID, AssignInput{Kind: "skill", SkillID: skillID}); err != nil {
		t.Fatal(err)
	}
}

// physicalStoreRoot returns the physical Store link root the distribution
// writes into raw link targets.
func physicalStoreRoot(t *testing.T, a *App) string {
	t.Helper()
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestDistributionLifecycle locks the full create/no-op/remove cycle:
// fresh inspection, dry-run planning without mutation, real distribution
// creating absolute links, idempotent reruns, and assignment removal
// removing only the provably managed link.
func TestDistributionLifecycle(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])

	st, err := a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "missing" || len(st.Items) != 1 {
		t.Fatalf("status: %+v", st)
	}
	if st.Items[0].Slug != "alpha" || st.Items[0].Desired != "present" || st.Items[0].Observed != "missing" {
		t.Fatalf("item: %+v", st.Items[0])
	}

	res, err := a.DistributeTarget(context.Background(), tv.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultSucceeded || res.Items[0].Result != distribution.OutcomeCreated {
		t.Fatalf("dry run: %+v", res)
	}
	if _, err := os.Lstat(tv.Path); !os.IsNotExist(err) {
		t.Fatal("dry run must not create the container")
	}

	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultSucceeded || res.Items[0].Result != distribution.OutcomeCreated {
		t.Fatalf("distribute: %+v", res)
	}
	raw, err := os.Readlink(filepath.Join(tv.Path, "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if raw != filepath.Join(physicalStoreRoot(t, a), "alpha") {
		t.Fatalf("raw target: %q", raw)
	}
	if _, err := os.Stat(filepath.Join(tv.Path, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("link does not resolve into the Store Skill: %v", err)
	}

	assignSkill(t, a, tv.ID, ids["beta"])
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("rerun: %+v", res)
	}
	if res.Items[0].Result != distribution.OutcomeNoOp || res.Items[1].Result != distribution.OutcomeCreated {
		t.Fatalf("rerun items: %+v", res.Items)
	}

	if _, err := a.UnassignTarget(tv.ID, "skill", ids["alpha"]); err != nil {
		t.Fatal(err)
	}
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != distribution.OutcomeRemoved || res.Items[1].Result != distribution.OutcomeNoOp {
		t.Fatalf("removal items: %+v", res.Items)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the removed link must be gone")
	}
	if _, err := os.Stat(filepath.Join(tv.Path, "beta", "SKILL.md")); err != nil {
		t.Fatal("the unrelated link must survive")
	}

	view, err := a.ShowTarget(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Distribution == nil || len(view.Distribution.Items) != 1 || view.Distribution.Items[0].Slug != "beta" {
		t.Fatalf("stored status: %+v", view.Distribution)
	}
	got := view.Distribution.Items[0]
	if !got.Managed || got.Observed != distribution.ObservedLinked || got.RawTarget == "" {
		t.Fatalf("stored observation incomplete: %+v", got)
	}
	if got.RawTarget != filepath.Join(physicalStoreRoot(t, a), "beta") {
		t.Fatalf("stored raw target: %q", got.RawTarget)
	}
}

// TestCancelledDistributePersistsFailedOutcome proves a cancelled create
// records failed with a CHECK-valid observed value, not an error code.
func TestCancelledDistributePersistsFailedOutcome(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := a.DistributeTarget(ctx, tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Result != distribution.OutcomeFailed {
		t.Fatalf("cancelled result: %+v", res)
	}
	view, err := a.ShowTarget(tv.ID)
	if err != nil || view.Distribution == nil || len(view.Distribution.Items) != 1 {
		t.Fatalf("stored: %+v, %v", view, err)
	}
	got := view.Distribution.Items[0]
	if got.LastResult != distribution.OutcomeFailed || got.Observed != distribution.ObservedMissing {
		t.Fatalf("stored failed observation: %+v", got)
	}
	if got.LastError == "" {
		t.Fatal("cancelled error must be recorded")
	}
}
