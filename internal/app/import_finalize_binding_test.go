package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
)

// TestImportSkillsCommittedJournalRetainedOnForeignLiveSwap proves the
// final live binding at the application boundary: a committed finalization
// whose live tree was swapped for a byte-identical foreign copy after the
// first live proof is refused by the second binding, the committed intent
// stays in the SQL journal, the operation evidence (staging and install
// proof) is preserved for conservative recovery, and the foreign live tree
// is never touched.
func TestImportSkillsCommittedJournalRetainedOnForeignLiveSwap(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	live := filepath.Join(a.StorePath, "alpha")
	body, err := os.ReadFile(filepath.Join(live, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookBeforeBaselineMoves {
			return
		}
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		// a byte-identical copy of the installed tree takes the live slot
		if err := os.WriteFile(filepath.Join(live, "SKILL.md"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	defer a.store.SetHook(nil)

	_, err = a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign live swap during committed finalization: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the committed intent must be retained in the journal: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(live, "SKILL.md")); rerr != nil || string(data) != string(body) {
		t.Fatalf("the byte-identical foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the install proof must be preserved for conservative recovery")
	}
	if _, err := os.Lstat(filepath.Join(live+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
}

// TestImportSkillsCommittedJournalRetainedOnLiveSwapAtRemoveOp proves the
// post-removal live binding at the application boundary: a committed
// finalization whose live tree was swapped for a byte-identical foreign
// copy at the strict-removal hook is refused after the operation children
// were removed but before the install proof is deleted, the committed
// intent stays in the SQL journal, the operation evidence (staging and
// install proof) is preserved for conservative recovery, and the foreign
// live tree is never touched.
func TestImportSkillsCommittedJournalRetainedOnLiveSwapAtRemoveOp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	op := importAndStopBeforeFinalize(t, a, src, "skills/alpha", "alpha")
	live := filepath.Join(a.StorePath, "alpha")
	body, err := os.ReadFile(filepath.Join(live, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	a.store.SetHook(func(p skillstore.HookPoint) {
		if p != skillstore.HookBeforeRemoveOp {
			return
		}
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(live, 0o755); err != nil {
			t.Fatal(err)
		}
		// a byte-identical copy of the installed tree takes the live slot
		if err := os.WriteFile(filepath.Join(live, "SKILL.md"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	defer a.store.SetHook(nil)

	_, err = a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("foreign live swap at the strict removal during committed finalization: %v", err)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the terminal finalized intent must be retained in the journal: %+v", ops)
	}
	if data, rerr := os.ReadFile(filepath.Join(live, "SKILL.md")); rerr != nil || string(data) != string(body) {
		t.Fatalf("the byte-identical foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "proof")); err != nil {
		t.Fatal("the install proof must be preserved for conservative recovery")
	}
	if _, err := os.Lstat(filepath.Join(live+".real", "SKILL.md")); err != nil {
		t.Fatal("the installed live tree must survive beside the foreign one")
	}
}
