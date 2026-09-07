package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

func TestRecoveryRemovePreparedIsolationCompletes(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.CreateIsolationDir(tv.Path, iso); err != nil {
		t.Fatal(err)
	}
	if err := distribution.IsolateInto(tv.Path, "demo", iso); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
		SlotName: iso, Phase: state.LinkPhasePrepared, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); !os.IsNotExist(err) {
		t.Fatal("the Managed Link must be removed")
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, iso)); !os.IsNotExist(err) {
		t.Fatal("the isolation directory must be removed")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the claim must be cleared")
	}
}

// TestRecoveryRemovePlannedEmptyIsolationRetries proves the crash window
// after CreateIsolationDir and before IsolateInto: phase is still planned,
// the private dir exists and is empty, and the Managed Link is still at
// the final slug. Recovery must isolate into that dir, not treat
// RemoveAbsent as "already gone".
func TestRecoveryRemovePlannedEmptyIsolationRetries(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.CreateIsolationDir(tv.Path, iso); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); err != nil {
		t.Fatal("precondition: Managed Link must still be at the final slug")
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, iso, distribution.IsolatedEntry)); !os.IsNotExist(err) {
		t.Fatal("precondition: IsolatedEntry must be absent")
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
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
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); !os.IsNotExist(err) {
		t.Fatal("the Managed Link must be removed")
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, iso)); !os.IsNotExist(err) {
		t.Fatal("the isolation directory must be removed")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the claim must be cleared")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("the intent must be cleared")
	}
}

func TestRecoveryRemovePlannedEmptyIsolationMissingSlug(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.CreateIsolationDir(tv.Path, iso); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tv.Path, "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
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
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("missing final slug must finalize the ledger")
	}
}

func TestRecoveryRemovePlannedEmptyIsolationChangedSlug(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.CreateIsolationDir(tv.Path, iso); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(tv.Path, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tv.Path, "demo"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
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
	if data, err := os.ReadFile(filepath.Join(tv.Path, "demo")); err != nil || string(data) != "user" {
		t.Fatalf("changed final slug must be left: %q, %v", data, err)
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("changed slug must relinquish the claim")
	}
}

func TestRecoveryRemoveUnprovenIsolationKept(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tv.Path, iso), []byte("not-ours"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
		SlotName: iso, Phase: state.LinkPhasePrepared, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if data, err := os.ReadFile(filepath.Join(tv.Path, iso)); err != nil || string(data) != "not-ours" {
		t.Fatalf("unproven isolation deleted: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); err != nil {
		t.Fatal("the Managed Link must survive")
	}
	intents, err := state.ListOpenLinkIntents(fresh.db)
	if err != nil || len(intents) != 1 {
		t.Fatalf("intent must be kept: %+v, %v", intents, err)
	}
}

func TestRecoveryLegacyEmptySlotCreateAndRemove(t *testing.T) {
	t.Parallel()
	a, targetID, skillID, tv := seedLinkFixture(t)
	importAllSkills(t, a, "other")
	otherID := skillIDBySlug(t, a, "other")
	assignSkill(t, a, targetID, otherID)
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: otherID, Action: "create",
		LinkPath: filepath.Join(tv.Path, "other"), RawTarget: filepath.Join(root, "other"),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatalf("legacy intents must converge: %+v", intents)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); !os.IsNotExist(err) {
		t.Fatal("legacy remove must take the Managed Link")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err == nil {
		t.Fatal("legacy create missing path must not register ownership")
	}
}
