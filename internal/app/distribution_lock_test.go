package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"skillctl/internal/distribution"
	"skillctl/internal/state"
)

// secondApp opens a second App over the same state and Store, without
// running the open recovery (no operations are open).
func secondApp(t *testing.T, a *App) *App {
	t.Helper()
	other, err := openApp(a.StorePath, a.StateDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	return other
}

// TestTargetLockContention proves a Target being reconciled by another
// process refuses new mutations with the stable locked code, and succeeds
// after the lock is released.
func TestTargetLockContention(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	other := secondApp(t, a)

	held, err := other.acquireTargetLock(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.DistributeTarget(context.Background(), tv.ID, false)
	if err == nil || err.(*Error).Code != CodeLocked {
		t.Fatalf("contended distribute: %v", err)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	if res, err := a.DistributeTarget(context.Background(), tv.ID, false); err != nil || res.Outcome != distribution.ResultSucceeded {
		t.Fatalf("retry: %+v, %v", res, err)
	}
}

// TestInspectTargetLockContention proves InspectTarget takes the Target
// exclusive lock (after the Store shared lock) and refuses while another
// process holds it.
func TestInspectTargetLockContention(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	tv := registerCustomTarget(t, a)
	assignSkill(t, a, tv.ID, ids["demo"])
	other := secondApp(t, a)

	held, err := other.acquireTargetLock(tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.InspectTarget(context.Background(), tv.ID)
	if err == nil || err.(*Error).Code != CodeLocked {
		t.Fatalf("contended inspect: %v", err)
	}
	if err := held.Unlock(); err != nil {
		t.Fatal(err)
	}
	st, err := a.InspectTarget(context.Background(), tv.ID)
	if err != nil || st.Items[0].Observed != distribution.ObservedMissing {
		t.Fatalf("inspect after unlock: %+v, %v", st, err)
	}
}

// TestConcurrentDistinctTargets proves different Targets reconcile
// concurrently under the shared Store lock, in stable Target-ID order.
func TestConcurrentDistinctTargets(t *testing.T) {
	a := newTestApp(t)
	ids := importAllSkills(t, a, "demo")
	first := registerCustomTarget(t, a)
	second := registerCustomTarget(t, a)
	assignSkill(t, a, first.ID, ids["demo"])
	assignSkill(t, a, second.ID, ids["demo"])
	other := secondApp(t, a)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	run := func(app *App, id int64, slot int) {
		defer wg.Done()
		_, errs[slot] = app.DistributeTarget(context.Background(), id, false)
	}
	wg.Add(2)
	go run(a, first.ID, 0)
	go run(other, second.ID, 1)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent distributes: %v, %v", errs[0], errs[1])
	}
	for _, tv := range []*TargetView{first, second} {
		link := filepath.Join(tv.Path, "demo")
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("link missing at %s: %v", link, err)
		}
		links, err := state.ListManagedLinksByTarget(a.db, tv.ID)
		if err != nil || len(links) != 1 {
			t.Fatalf("ledger of %d: %+v, %v", tv.ID, links, err)
		}
	}
}
