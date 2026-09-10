package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/distribution"
)

// TestLinkAndUnlinkTargetSkill locks the single-Skill link lifecycle:
// linking one desired Skill creates just its Managed Link (and the
// container), a second link is an idempotent no-op, unlinking removes only
// that link while keeping the Assignment, and a second unlink is a no-op.
func TestLinkAndUnlinkTargetSkill(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])
	assignSkill(t, a, tv.ID, ids["beta"])

	st, err := a.LinkTargetSkill(context.Background(), tv.ID, ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	alpha := itemBySlug(t, st, "alpha")
	if alpha.Observed != distribution.ObservedLinked || !alpha.Managed {
		t.Fatalf("alpha not linked: %+v", alpha)
	}
	if beta := itemBySlug(t, st, "beta"); beta.Observed != distribution.ObservedMissing {
		t.Fatalf("beta must stay unlinked: %+v", beta)
	}
	if _, err := os.Stat(filepath.Join(tv.Path, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("link does not resolve into the Store Skill: %v", err)
	}

	// Linking again is a no-op that keeps the link.
	st, err = a.LinkTargetSkill(context.Background(), tv.ID, ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	if itemBySlug(t, st, "alpha").Observed != distribution.ObservedLinked {
		t.Fatal("re-link must remain linked")
	}

	// Unlinking removes the link but leaves the Assignment (still desired).
	st, err = a.UnlinkTargetSkill(context.Background(), tv.ID, ids["alpha"])
	if err != nil {
		t.Fatal(err)
	}
	alpha = itemBySlug(t, st, "alpha")
	if alpha.Desired != distribution.DesiredPresent || alpha.Observed != distribution.ObservedMissing {
		t.Fatalf("alpha must be desired but unlinked: %+v", alpha)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the unlinked link must be gone")
	}

	// Unlinking again is a no-op.
	if _, err := a.UnlinkTargetSkill(context.Background(), tv.ID, ids["alpha"]); err != nil {
		t.Fatal(err)
	}

	// A later distribution recreates it, proving the Assignment survived.
	if _, err := a.LinkTargetSkill(context.Background(), tv.ID, ids["alpha"]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tv.Path, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("re-link failed: %v", err)
	}
}

// TestLinkNonDesiredSkillConflicts proves a Skill that is not assigned to
// the Target cannot be linked.
func TestLinkNonDesiredSkillConflicts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha", "beta")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])

	_, err := a.LinkTargetSkill(context.Background(), tv.ID, ids["beta"])
	if err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("want CodeTargetConflict, got %v", err)
	}
}

// TestLinkConflictOnForeignEntry proves an existing foreign entry at the
// link path blocks the link with a conflict rather than overwriting it.
func TestLinkConflictOnForeignEntry(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "alpha")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["alpha"])

	if err := os.MkdirAll(tv.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(tv.Path, "alpha")
	if err := os.WriteFile(foreign, []byte("not ours"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := a.LinkTargetSkill(context.Background(), tv.ID, ids["alpha"])
	if err == nil || err.(*Error).Code != CodeTargetConflict {
		t.Fatalf("want CodeTargetConflict, got %v", err)
	}
	if data, rerr := os.ReadFile(foreign); rerr != nil || string(data) != "not ours" {
		t.Fatalf("foreign entry must be preserved: %q, %v", data, rerr)
	}
}

func itemBySlug(t *testing.T, st *DistributionStatus, slug string) DistributionItemView {
	t.Helper()
	for _, it := range st.Items {
		if it.Slug == slug {
			return it
		}
	}
	t.Fatalf("no item for slug %q in %+v", slug, st.Items)
	return DistributionItemView{}
}
