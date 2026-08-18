package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/distribution"
)

// TestDistributionConflictPreservedAndAdopted proves an unmanaged entry is
// never overwritten, a correct unmanaged symlink is never adopted
// implicitly, and explicit adoption records the existing link without
// rewriting it.
func TestDistributionConflictPreservedAndAdopted(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	container := tv.Path
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}

	before := []byte("mine\000content")
	if err := os.WriteFile(filepath.Join(container, "demo"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Items[0].Observed != "conflict" || st.Items[0].NodeKind != "file" || st.Items[0].Adoptable {
		t.Fatalf("file conflict: %+v", st.Items[0])
	}
	res, err := a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != distribution.ResultBlocked || res.Items[0].Result != distribution.OutcomeBlockedConflict {
		t.Fatalf("blocked: %+v", res)
	}
	if after, err := os.ReadFile(filepath.Join(container, "demo")); err != nil || string(after) != string(before) {
		t.Fatalf("foreign file changed: %q, %v", after, err)
	}

	if err := os.Remove(filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(physicalStoreRoot(t, a), "demo")
	if err := os.Symlink(expected, filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	st, err = a.InspectTarget(context.Background(), tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Items[0].Observed != "conflict" || !st.Items[0].Adoptable || st.Items[0].Managed {
		t.Fatalf("adoptable conflict: %+v", st.Items[0])
	}
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != distribution.OutcomeBlockedConflict {
		t.Fatalf("must not auto-adopt: %+v", res.Items[0])
	}

	ar, err := a.AdoptTargetLink(context.Background(), tv.ID, ids["demo"])
	if err != nil {
		t.Fatal(err)
	}
	if ar.Result != distribution.OutcomeAdopted || ar.RawTarget != expected {
		t.Fatalf("adopt: %+v", ar)
	}
	view, err := a.ShowTarget(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Distribution == nil || len(view.Distribution.Items) != 1 {
		t.Fatalf("stored after adopt: %+v", view.Distribution)
	}
	got := view.Distribution.Items[0]
	if got.Observed != "linked" || !got.Managed || got.LastResult != distribution.OutcomeAdopted {
		t.Fatalf("adopted stored observation: %+v", got)
	}
	res, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Result != distribution.OutcomeNoOp {
		t.Fatalf("adopted rerun: %+v", res.Items[0])
	}
}

// TestAdoptionEligibility proves the adopt gate: wrong targets, files, and
// non-desired Skills are refused.
func TestAdoptionEligibility(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo", "other")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	container := tv.Path
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AdoptTargetLink(context.Background(), tv.ID, ids["demo"]); err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("wrong target: %v", err)
	}
	if err := os.Remove(filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AdoptTargetLink(context.Background(), tv.ID, ids["demo"]); err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("file: %v", err)
	}
	if err := os.Remove(filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(physicalStoreRoot(t, a), "other"), filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AdoptTargetLink(context.Background(), tv.ID, ids["demo"]); err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("wrong slug resolution: %v", err)
	}
	if _, err := a.AdoptTargetLink(context.Background(), tv.ID, ids["other"]); err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("not desired: %v", err)
	}
}
