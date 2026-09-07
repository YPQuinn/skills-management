package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

func TestProcessCrashCreateBeforePublication(t *testing.T) {
	for _, phase := range []string{"dist-create-planned", "dist-create-prepared"} {
		t.Run(phase, func(t *testing.T) {
			fx := seedDistCreateCrash(t)
			runCrashChild(t, fx, phase, crashHelperExit)
			if _, err := os.Lstat(filepath.Join(fx.targetPath, "demo")); !os.IsNotExist(err) {
				t.Fatalf("link published before crash: %v", err)
			}
			fresh := reopenCrash(t, fx)
			intents, err := state.ListOpenLinkIntents(fresh.db)
			if err != nil || len(intents) != 0 {
				t.Fatalf("unfinished staging intent: %+v, %v", intents, err)
			}
			if _, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("unpublished link became owned: %v", err)
			}
			entries, err := os.ReadDir(fx.targetPath)
			if err != nil && !os.IsNotExist(err) || len(entries) != 0 {
				t.Fatalf("staging survived cleanup: %+v, %v", entries, err)
			}
			res, err := fresh.DistributeTarget(context.Background(), fx.targetID, false)
			if err != nil || res.Outcome != distribution.ResultSucceeded {
				t.Fatalf("retry after recovery: %+v, %v", res, err)
			}
		})
	}
}

func TestProcessCrashCreateUnprovenStagingIsPreserved(t *testing.T) {
	fx := seedDistCreateCrash(t)
	runCrashChild(t, fx, "dist-create-staged", crashHelperExit)
	db, err := state.Open(fx.db)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	intents, err := state.ListOpenLinkIntents(db)
	if err != nil || len(intents) != 1 || intents[0].SlotName == "" || intentProof(intents[0]).Proven() {
		t.Fatalf("unproven receipt missing: %+v, %v", intents, err)
	}
	staged := filepath.Join(fx.targetPath, intents[0].SlotName, distribution.IsolatedEntry)
	before, err := os.Readlink(staged)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		fresh, err := New(fx.store, fx.db)
		if fresh != nil {
			fresh.Close()
		}
		if !isCode(err, CodeRecovery) {
			t.Fatalf("unknown staging must block recovery: %v", err)
		}
		if got, err := os.Readlink(staged); err != nil || got != before {
			t.Fatalf("unproven staging lost: %q, %v", got, err)
		}
		if _, err := os.Lstat(filepath.Join(fx.targetPath, "demo")); !os.IsNotExist(err) {
			t.Fatalf("unknown staging was published: %v", err)
		}
	}
	intents, err = state.ListOpenLinkIntents(db)
	if err != nil || len(intents) != 1 {
		t.Fatalf("unproven receipt discarded: %+v, %v", intents, err)
	}
}

func TestDistributionCreateRecordsProofAndPreservesConcurrentEntry(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	var proof distribution.LinkProof
	a.linkMutateHook = func(phase string) {
		if phase != "create-prepared" {
			return
		}
		intents, err := state.ListOpenLinkIntents(a.db)
		if err != nil || len(intents) != 1 {
			t.Fatalf("receipt: %+v, %v", intents, err)
		}
		proof = intentProof(intents[0])
		got, err := distribution.ProbeSymlink(filepath.Join(tv.Path, intents[0].SlotName), distribution.IsolatedEntry)
		if err != nil || !proof.Matches(got) {
			t.Fatalf("durable staging proof: %+v, %v", got, err)
		}
		if err := os.Symlink(proof.Raw, filepath.Join(tv.Path, "demo")); err != nil {
			t.Fatal(err)
		}
	}
	res, err := a.DistributeTarget(context.Background(), tv.ID, false)
	if err != nil || len(res.Items) != 1 || res.Items[0].Result != distribution.OutcomeBlockedConflict {
		t.Fatalf("concurrent entry: %+v, %v", res, err)
	}
	if got, err := os.Readlink(filepath.Join(tv.Path, "demo")); err != nil || got != proof.Raw {
		t.Fatalf("external entry was lost: %q, %v", got, err)
	}
	if _, err := state.GetManagedLink(a.db, tv.ID, ids["demo"]); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("external entry became owned: %v", err)
	}
	intents, err := state.ListOpenLinkIntents(a.db)
	if err != nil || len(intents) != 0 {
		t.Fatalf("intent leak: %+v, %v", intents, err)
	}
	entries, err := os.ReadDir(tv.Path)
	if err != nil || len(entries) != 1 || entries[0].Name() != "demo" {
		t.Fatalf("staging leak: %+v, %v", entries, err)
	}
}

func TestRecoveryCreatePreservesReplacedStaging(t *testing.T) {
	fx := seedDistCreateCrash(t)
	runCrashChild(t, fx, "dist-create-prepared", crashHelperExit)
	db, err := state.Open(fx.db)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	intents, err := state.ListOpenLinkIntents(db)
	if err != nil || len(intents) != 1 {
		t.Fatalf("receipt: %+v, %v", intents, err)
	}
	staged := filepath.Join(fx.targetPath, intents[0].SlotName, distribution.IsolatedEntry)
	raw := intents[0].RawTarget
	if err := os.Symlink(raw, staged+".replacement"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staged+".replacement", staged); err != nil {
		t.Fatal(err)
	}
	fresh, err := New(fx.store, fx.db)
	if fresh != nil {
		fresh.Close()
	}
	if !isCode(err, CodeRecovery) {
		t.Fatalf("replaced staging must block recovery: %v", err)
	}
	if got, err := os.Readlink(staged); err != nil || got != raw {
		t.Fatalf("replacement lost: %q, %v", got, err)
	}
	intents, err = state.ListOpenLinkIntents(db)
	if err != nil || len(intents) != 1 {
		t.Fatalf("receipt discarded: %+v, %v", intents, err)
	}
}

func TestProcessCrashCreateAfterPublicationCleansStaging(t *testing.T) {
	fx := seedDistCreateCrash(t)
	runCrashChild(t, fx, "dist-create", crashHelperExit)
	fresh := reopenCrash(t, fx)
	ledger, err := state.GetManagedLink(fresh.db, fx.targetID, fx.skillID)
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.VerifyLink(fx.targetPath, "demo", ledgerProof(ledger)); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(fx.targetPath)
	if err != nil || len(entries) != 1 || entries[0].Name() != "demo" {
		t.Fatalf("staging leak after published-create recovery: %+v, %v", entries, err)
	}
	intents, err := state.ListOpenLinkIntents(fresh.db)
	if err != nil || len(intents) != 0 {
		t.Fatalf("receipt leak: %+v, %v", intents, err)
	}
}
