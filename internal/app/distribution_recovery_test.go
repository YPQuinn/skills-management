package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// seedLinkFixture imports one Skill, registers one Target, assigns the
// Skill, and distributes it.
func seedLinkFixture(t *testing.T) (*App, int64, int64, *TargetView) {
	t.Helper()
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	if res, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil || res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("seed distribute: %+v, %v", res, err)
	}
	return a, tv.ID, ids["demo"], tv
}

// skillIDBySlug returns one imported Skill id by slug.
func skillIDBySlug(t *testing.T, a *App, slug string) int64 {
	t.Helper()
	items, err := a.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range items {
		if s.Slug == slug {
			return s.ID
		}
	}
	t.Fatalf("skill %q not found", slug)
	return 0
}

// TestRecoveryPendingCreateMissingPath proves a pending create with a
// missing path clears the intent and leaves the item missing.
func TestRecoveryPendingCreateMissingPath(t *testing.T) {
	a, targetID, _, tv := seedLinkFixture(t)
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
	if intents, err := state.ListOpenLinkIntents(fresh.db); err != nil || len(intents) != 0 {
		t.Fatalf("intent must be cleared: %+v, %v", intents, err)
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "other")); !os.IsNotExist(err) {
		t.Fatal("no link may have been created")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err == nil {
		t.Fatal("no ownership may be registered")
	}
}

// TestRecoveryPendingCreateUnprovenIntentIsNotAdopted proves a pending
// create without a persisted identity cannot claim a matching symlink.
func TestRecoveryPendingCreateUnprovenIntentIsNotAdopted(t *testing.T) {
	a, targetID, _, tv := seedLinkFixture(t)
	importAllSkills(t, a, "other")
	otherID := skillIDBySlug(t, a, "other")
	assignSkill(t, a, targetID, otherID)
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(root, "other")
	if err := os.Symlink(raw, filepath.Join(tv.Path, "other")); err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: otherID, Action: "create",
		LinkPath: filepath.Join(tv.Path, "other"), RawTarget: raw,
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
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err == nil {
		t.Fatal("an unproven create intent must not register ownership")
	}
	if _, err := os.Lstat(filepath.Join(tv.Path, "other")); err != nil {
		t.Fatal("the matching symlink must be preserved")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("conflict must end the create intent")
	}
}

// TestRecoveryPendingCreateProvenIdentityCompletes proves a create intent
// that recorded the symlink identity at create time completes the ledger.
func TestRecoveryPendingCreateProvenIdentityCompletes(t *testing.T) {
	a, targetID, _, tv := seedLinkFixture(t)
	importAllSkills(t, a, "other")
	otherID := skillIDBySlug(t, a, "other")
	assignSkill(t, a, targetID, otherID)
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(root, "other")
	proof, err := distribution.CreateLink(tv.Path, "other", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: otherID, Action: "create",
		LinkPath: filepath.Join(tv.Path, "other"), RawTarget: raw,
		LinkDev: proof.Dev, LinkIno: proof.Ino, LinkMtime: proof.Mtime,
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
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err != nil {
		t.Fatalf("ownership must be registered: %v", err)
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("intent must be cleared")
	}
}

// TestRecoveryPendingCreateForeignEntry proves a pending create with any
// other entry preserves it as a conflict.
func TestRecoveryPendingCreateForeignEntry(t *testing.T) {
	a, targetID, _, tv := seedLinkFixture(t)
	importAllSkills(t, a, "other")
	otherID := skillIDBySlug(t, a, "other")
	assignSkill(t, a, targetID, otherID)
	// A foreign file appears at the slug with the intent still open.
	if err := os.WriteFile(filepath.Join(tv.Path, "other"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := a.storeLinkRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: otherID, Action: "create",
		LinkPath: filepath.Join(tv.Path, "other"), RawTarget: filepath.Join(root, "other"),
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
	if data, err := os.ReadFile(filepath.Join(tv.Path, "other")); err != nil || string(data) != "mine" {
		t.Fatalf("foreign entry changed: %q, %v", data, err)
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, otherID); err == nil {
		t.Fatal("no ownership may be registered for a foreign entry")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("conflict must end the create intent")
	}
}

// TestRecoveryPendingRemoveRetriesRemoval proves a pending remove with the
// unchanged Managed Link retries the removal.
func TestRecoveryPendingRemoveRetriesRemoval(t *testing.T) {
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
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
	if _, err := os.Lstat(filepath.Join(tv.Path, "demo")); !os.IsNotExist(err) {
		t.Fatal("the Managed Link must be removed")
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the claim must be cleared")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("intent must be cleared")
	}
}

// TestRecoveryPendingRemoveMissingPath proves a pending remove with a
// missing path completes the ledger cleanup.
func TestRecoveryPendingRemoveMissingPath(t *testing.T) {
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	// Crash between the filesystem removal and the finalization.
	if err := os.Remove(filepath.Join(tv.Path, "demo")); err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
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
		t.Fatal("the claim must be cleared")
	}
	if intents, _ := state.ListOpenLinkIntents(fresh.db); len(intents) != 0 {
		t.Fatal("intent must be cleared")
	}
}

// TestRecoveryPendingRemoveChangedEntry proves a pending remove with a
// replaced entry leaves it untouched and records ownership_lost.
func TestRecoveryPendingRemoveChangedEntry(t *testing.T) {
	a, targetID, skillID, _ := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
		SlotName: iso, Phase: state.LinkPhasePlanned, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	// The entry is replaced after the intent was recorded.
	if err := os.Remove(link.LinkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link.LinkPath, []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if data, err := os.ReadFile(link.LinkPath); err != nil || string(data) != "user" {
		t.Fatalf("replacement changed: %q, %v", data, err)
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the invalid claim must be relinquished")
	}
	items, err := state.ListDistributionItems(fresh.db, targetID)
	if err != nil || len(items) != 1 || items[0].LastResult != distribution.OutcomeOwnershipLost {
		t.Fatalf("ownership_lost must be recorded: %+v, %v", items, err)
	}
}

// TestRecoveryPendingRemoveSameRawReplacement proves a pending remove
// whose slug was replaced with a new symlink of the same raw target
// leaves the replacement and records ownership_lost.
func TestRecoveryPendingRemoveSameRawReplacement(t *testing.T) {
	a, targetID, skillID, tv := seedLinkFixture(t)
	link, err := state.GetManagedLink(a.db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	iso, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.InsertLinkIntent(a.db, state.LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: link.LinkPath, RawTarget: link.RawTarget,
		LinkDev: link.LinkDev, LinkIno: link.LinkIno, LinkMtime: link.LinkMtime,
		SlotName: iso, Phase: state.LinkPhasePlanned, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tv.Path, "demo")
	sib := path + ".new"
	if err := os.Symlink(link.RawTarget, sib); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sib, path); err != nil {
		t.Fatal(err)
	}
	a.Close()

	fresh, err := New(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got, err := os.Readlink(path); err != nil || got != link.RawTarget {
		t.Fatalf("replacement must be preserved: %q, %v", got, err)
	}
	if _, err := state.GetManagedLink(fresh.db, targetID, skillID); err == nil {
		t.Fatal("the invalid claim must be relinquished")
	}
	items, err := state.ListDistributionItems(fresh.db, targetID)
	if err != nil || len(items) != 1 || items[0].LastResult != distribution.OutcomeOwnershipLost {
		t.Fatalf("ownership_lost must be recorded: %+v, %v", items, err)
	}
}
