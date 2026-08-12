package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/state"
)

// TestImportSkillsFreshAppRecoversPreStageReplaceIntent proves the
// pre-Stage replace crash window at the application boundary: a replace
// intent persisted right before Stage ran has no operation evidence at
// all, so recovery converts the pending intent to the abort protocol —
// Store Abort verifies the operation/recovery absence without touching
// live content, the row is CAS-marked aborted, and the full-identity/phase
// CAS clears it. The old live tree is preserved and the already-bound
// entry reports already_imported.
func TestImportSkillsFreshAppRecoversPreStageReplaceIntent(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("initial import: %+v", first.Items[0])
	}
	detail, err := state.GetSkillDetailBySlug(a.db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	_, digest := materializeTo(t, a, src, "skills/alpha")
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindReplace,
		OldDigest: detail.Skill.StoreDigest, NewDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	// crash before Stage: no staging, no recovery slot
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("test setup must leave no staging")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "recovery", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("test setup must leave no recovery slot")
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != detail.Skill.ID {
		t.Fatalf("item after pre-Stage recovery: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the pre-Stage replace intent must be cleared: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil ||
		string(data) != "---\nname: Alpha\ndescription: desc\n---\n# Alpha\n" {
		t.Fatalf("the old live tree must be preserved: %q, %v", data, err)
	}
}

// TestImportSkillsFreshAppBlocksPendingImportWithForeignRecoveryEvidence
// proves the abort protocol verifies operation/recovery absence before
// anything is cleared: a pending import with no staging but a recovery
// slot (evidence no import can ever create) is refused, the row stays in
// its durable aborted phase (the approved ordering marks aborted before
// Store Abort verification), and the evidence is preserved.
func TestImportSkillsFreshAppBlocksPendingImportWithForeignRecoveryEvidence(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	_, digest := materializeTo(t, a, src, "skills/alpha")
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	// contradictory evidence for an import: a recovery slot appears with
	// no staging
	rec := filepath.Join(a.StorePath, ".skillctl", "recovery", itoa(op.ID))
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rec, "old"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign recovery evidence on a pending import: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseAborted {
		t.Fatalf("the intent must stay durably aborted: %+v", ops)
	}
	if _, err := os.Lstat(rec); err != nil {
		t.Fatal("the foreign recovery evidence must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may be created")
	}
}

// TestImportSkillsFreshAppBlocksPendingEmptyRowWithTerminalTombstone proves
// the deterministic terminal tombstone blocks the abort protocol for an
// evidence-free pending row: recovery marks the row aborted (approved
// ordering), Store Abort then refuses on the tombstone, and the row and
// the object are retained.
func TestImportSkillsFreshAppBlocksPendingEmptyRowWithTerminalTombstone(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	if res := importSkills(t, a, src.ID, "skills/alpha"); res.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", res.Items[0])
	}
	_, digest := materializeTo(t, a, src, "skills/alpha")
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		Slug: "beta", Kind: skillstore.KindImport, NewDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	// a terminal tombstone with no operation directory: terminal-cleanup
	// evidence a pre-install refusal can never create
	tomb := filepath.Join(a.StorePath, ".skillctl", "staging", ".skillctl-term-"+itoa(op.ID))
	if err := os.MkdirAll(tomb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tomb, "leftover"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("terminal tombstone on a pending-empty row: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseAborted {
		t.Fatalf("the row must stay durably aborted: %+v", ops)
	}
	if _, err := os.Lstat(tomb); err != nil {
		t.Fatal("the terminal tombstone must be preserved")
	}
}

// TestImportSkillsFreshAppBlocksAbortedRowWithTerminalTombstone proves the
// same refusal for an already-aborted row: Store Abort sees the tombstone
// and recovery fails with the row and the object retained.
func TestImportSkillsFreshAppBlocksAbortedRowWithTerminalTombstone(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	if res := importSkills(t, a, src.ID, "skills/alpha"); res.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", res.Items[0])
	}
	_, digest := materializeTo(t, a, src, "skills/alpha")
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		Slug: "beta", Kind: skillstore.KindImport, NewDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.MarkOperationAborted(a.db, op); err != nil {
		t.Fatal(err)
	}
	op.Phase = skillstore.PhaseAborted
	tomb := filepath.Join(a.StorePath, ".skillctl", "staging", ".skillctl-term-"+itoa(op.ID))
	if err := os.MkdirAll(tomb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tomb, "leftover"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("terminal tombstone on an aborted row: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseAborted {
		t.Fatalf("the aborted row must be retained: %+v", ops)
	}
	if _, err := os.Lstat(tomb); err != nil {
		t.Fatal("the terminal tombstone must be preserved")
	}
}

// TestImportImmediateAbortMarksAbortedBeforeAbortVerification proves the
// approved durable ordering on the immediate pre-install abort path: by
// the time Store Abort runs its verification (observed at the layout
// hook), the row is already durably aborted through the full-identity CAS.
func TestImportImmediateAbortMarksAbortedBeforeAbortVerification(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	unmanaged := filepath.Join(a.StorePath, "alpha")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 0
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterLayoutOpen {
			return
		}
		// Stage, Install, and DiscardStaging open the layout first; the
		// abort verification opens it fourth, at which point the row must
		// already be durably aborted.
		n++
		if n != 4 {
			return
		}
		ops := openOperations(t, a)
		if len(ops) != 1 || ops[0].Phase != skillstore.PhaseAborted {
			t.Fatalf("the row must be aborted before Store Abort verifies: %+v", ops)
		}
	})
	defer a.store.SetHook(nil)

	res := importSkills(t, a, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeConflict {
		t.Fatalf("aborted unmanaged import: %+v", res.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("the aborted intent must be cleared: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(unmanaged, "mine.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
}

// TestImportFreshAppPreStageMarksAbortedBeforeAbortVerification proves the
// approved durable ordering on fresh-App recovery of an evidence-free
// pending row: the row is already durably aborted when Store Abort runs
// its verification, and the recovery converges.
func TestImportFreshAppPreStageMarksAbortedBeforeAbortVerification(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	detail, err := state.GetSkillDetailBySlug(a.db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	_, digest := materializeTo(t, a, src, "skills/alpha")
	op, err := state.InsertOperation(a.db, skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindReplace,
		OldDigest: detail.Skill.StoreDigest, NewDigest: digest,
	})
	if err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	n := 0
	fresh.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookAfterLayoutOpen {
			return
		}
		// PrepareRestore opens the layout first; Store Abort's verification
		// opens it second, when the row must already be durably aborted.
		n++
		if n != 2 {
			return
		}
		ops := openOperations(t, fresh)
		if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseAborted {
			t.Fatalf("the row must be aborted before Store Abort verifies: %+v", ops)
		}
	})
	defer fresh.store.SetHook(nil)

	res := importSkills(t, fresh, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != detail.Skill.ID {
		t.Fatalf("item after pre-Stage recovery: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the pre-Stage intent must be cleared: %+v", ops)
	}
}
