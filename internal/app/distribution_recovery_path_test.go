package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// TestRecoveryRemoveIgnoresPoisonedLinkPath proves recovery re-reads the
// Target and Skill and never follows a recorded intent.LinkPath. A decoy
// symlink at the poisoned path is left untouched; the real Target entry is
// the one that is removed.
func TestRecoveryRemoveIgnoresPoisonedLinkPath(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	decoyDir := filepath.Join(t.TempDir(), "decoy")
	if err := os.MkdirAll(decoyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(decoyDir, "demo")
	if err := os.Symlink(link.RawTarget, decoy); err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: decoy, RawTarget: link.RawTarget,
		SlotName: iso, Phase: state.LinkPhasePlanned, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if _, err := os.Lstat(decoy); err != nil {
		t.Fatalf("decoy must survive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); !os.IsNotExist(err) {
		t.Fatal("the real Managed Link must be removed")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the claim must be cleared")
	}
}

// TestRecoveryCreateIgnoresPoisonedLinkPath proves a pending create whose
// recorded LinkPath already has the expected symlink does not register
// ownership from that path. Recovery inspects the live Target slug instead.
func TestRecoveryCreateIgnoresPoisonedLinkPath(t *testing.T) {
	t.Parallel()
	a, targetID, _, tv := seedLinkFixture(t)
	importAllSkills(t, a, "other")
	otherID := skillIDBySlug(t, a, "other")
	assignSkill(t, a, targetID, otherID)
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(root, "other")
	decoyDir := filepath.Join(t.TempDir(), "decoy")
	if err := os.MkdirAll(decoyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(raw, filepath.Join(decoyDir, "other")); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: otherID, Action: "create",
		LinkPath: filepath.Join(decoyDir, "other"), RawTarget: raw,
		Phase: state.LinkPhasePlanned, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if _, err := os.Lstat(filepath.Join(decoyDir, "other")); err != nil {
		t.Fatalf("decoy must survive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "other")); !os.IsNotExist(err) {
		t.Fatal("no link may be created at the Target")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err == nil {
		t.Fatal("ownership must not be registered from a poisoned path")
	}
}
