package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/sync"
)

// TestSyncBatchOneCoherentObservation locks decision 05: one batch uses
// exactly one fresh Source observation for every bound Skill.
func TestSyncBatchOneCoherentObservation(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{
		"skills/alpha": "Alpha",
		"skills/beta":  "Beta",
	})
	if res := importSkills(t, a, src.ID, "skills/alpha"); len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("import alpha: %+v", res.Items)
	}
	if res := importSkills(t, a, src.ID, "skills/beta"); len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("import beta: %+v", res.Items)
	}
	var observes int
	a.observer = countingObserver{Observer: a.observer, n: &observes}

	rewriteSourceFile(t, src, "skills/alpha", "notes.md", "alpha v2\n")
	rewriteSourceFile(t, src, "skills/beta", "notes.md", "beta v2\n")
	result, err := a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observes != 1 {
		t.Fatalf("batch used %d observations, want exactly 1", observes)
	}
	if len(result.Items) != 2 || result.Summary.Total != 2 || result.Summary.Updated != 2 {
		t.Fatalf("batch result: %+v", result)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "alpha", "notes.md")); err != nil || string(data) != "alpha v2\n" {
		t.Fatalf("alpha content: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "beta", "notes.md")); err != nil || string(data) != "beta v2\n" {
		t.Fatalf("beta content: %q, %v", data, err)
	}
	// The second batch (in sync) is still exactly one observation.
	result, err = a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observes != 2 || result.Summary.NoOp != 2 {
		t.Fatalf("second batch: %d observations, %+v", observes, result.Summary)
	}
}

// TestSyncBatchPerItemIsolation locks decision 05: one Skill's outcome
// never rolls back a sibling's success, and every item outcome is reported.
func TestSyncBatchPerItemIsolation(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{
		"skills/auto":   "Auto",
		"skills/edited": "Edited",
	})
	if res := importSkills(t, a, src.ID, "skills/auto"); len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("import auto: %+v", res.Items)
	}
	if res := importSkills(t, a, src.ID, "skills/edited"); len(res.Items) != 1 || res.Items[0].Status != StatusImported {
		t.Fatalf("import edited: %+v", res.Items)
	}
	rewriteSourceFile(t, src, "skills/auto", "notes.md", "auto v2\n")
	rewriteSourceFile(t, src, "skills/edited", "notes.md", "edited v2\n")
	rewriteStoreFile(t, a, "edited", "local.txt", "local edit\n")

	result, err := a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]SyncItemResult{}
	for _, it := range result.Items {
		bySlug[it.Slug] = it
	}
	if bySlug["auto"].Result != sync.ResultUpdated {
		t.Fatalf("auto must update independently: %+v", bySlug["auto"])
	}
	if bySlug["edited"].Result != sync.ResultSkipped || bySlug["edited"].Status != sync.StatusConflict {
		t.Fatalf("edited must skip as conflict: %+v", bySlug["edited"])
	}
	if result.Summary.Updated != 1 || result.Summary.Skipped != 1 {
		t.Fatalf("summary: %+v", result.Summary)
	}
	// The skipped Skill's local edit survives byte for byte.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "edited", "local.txt")); err != nil || string(data) != "local edit\n" {
		t.Fatalf("edited content touched: %q, %v", data, err)
	}
	// The updated Skill received the upstream change and nothing else.
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "auto", "notes.md")); err != nil || string(data) != "auto v2\n" {
		t.Fatalf("auto content: %q, %v", data, err)
	}
}

// TestSyncBatchUnavailable locks the batch behavior for an unreachable
// Source: every bound Skill is reported blocked with its last relationship
// retained and marked stale, and the Store is never touched.
func TestSyncBatchUnavailable(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	res := importSkills(t, a, src.ID, "skills/alpha")
	if res.Items[0].Status != StatusImported {
		t.Fatalf("import: %+v", res.Items)
	}
	if err := os.RemoveAll(src.Location); err != nil {
		t.Fatal(err)
	}
	result, err := a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Result != sync.ResultBlocked || !result.Items[0].Stale {
		t.Fatalf("unavailable batch: %+v", result)
	}
	if result.Items[0].Status != sync.StatusInSync {
		t.Fatalf("relationship retained: %+v", result.Items[0])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Store content must be untouched: %v", err)
	}
}

// TestSyncBatchSkipsMissing locks the missing-entry outcomes inside a
// batch: source_missing blocks and store_missing skips.
func TestSyncBatchSkipsMissing(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{
		"skills/gone":   "Gone",
		"skills/hollow": "Hollow",
	})
	if res := importSkills(t, a, src.ID, "skills/gone"); res.Items[0].Status != StatusImported {
		t.Fatalf("import gone: %+v", res.Items)
	}
	if res := importSkills(t, a, src.ID, "skills/hollow"); res.Items[0].Status != StatusImported {
		t.Fatalf("import hollow: %+v", res.Items)
	}
	if err := os.RemoveAll(filepath.Join(src.Location, "skills", "gone")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(a.StorePath, "hollow")); err != nil {
		t.Fatal(err)
	}
	result, err := a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]SyncItemResult{}
	for _, it := range result.Items {
		bySlug[it.Slug] = it
	}
	if bySlug["gone"].Result != sync.ResultBlocked || bySlug["gone"].Status != sync.StatusSourceMissing {
		t.Fatalf("gone: %+v", bySlug["gone"])
	}
	if bySlug["hollow"].Result != sync.ResultSkipped || bySlug["hollow"].Status != sync.StatusStoreMissing {
		t.Fatalf("hollow: %+v", bySlug["hollow"])
	}
}

// TestSyncBatchBlocksAfterUnresolvedReplace locks the recovery-blocks-
// writes rule inside a synchronization batch: when a source_changed update
// leaves an unresolved Store operation (its terminal deletion failed), the
// batch reports that item failed/CodeRecovery, stops every later Store
// write with failed/CodeRecovery, and the next batch's recovery converges.
func TestSyncBatchBlocksAfterUnresolvedReplace(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{
		"skills/alpha": "Alpha",
		"skills/beta":  "Beta",
	})
	if res := importSkills(t, a, src.ID, "skills/alpha"); res.Items[0].Status != StatusImported {
		t.Fatalf("import alpha: %+v", res.Items)
	}
	if res := importSkills(t, a, src.ID, "skills/beta"); res.Items[0].Status != StatusImported {
		t.Fatalf("import beta: %+v", res.Items)
	}
	rewriteSourceFile(t, src, "skills/alpha", "notes.md", "alpha v2\n")
	rewriteSourceFile(t, src, "skills/beta", "notes.md", "beta v2\n")

	// The failpoint: the terminal row deletion of the committed alpha
	// operation fails, leaving an unresolved finalized intent.
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }
	result, err := a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]SyncItemResult{}
	for _, it := range result.Items {
		bySlug[it.Slug] = it
	}
	if bySlug["alpha"].Result != sync.ResultFailed || bySlug["alpha"].ErrorCode != CodeRecovery {
		t.Fatalf("alpha must fail recovery: %+v", bySlug["alpha"])
	}
	if bySlug["beta"].Result != sync.ResultFailed || bySlug["beta"].ErrorCode != CodeRecovery {
		t.Fatalf("beta must be blocked by the unresolved operation: %+v", bySlug["beta"])
	}
	if result.Summary.Failed != 2 {
		t.Fatalf("summary: %+v", result.Summary)
	}
	// Beta's live content was never touched while the block held.
	if _, err := os.Lstat(filepath.Join(a.StorePath, "beta", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("beta was written despite the unresolved operation: %v", err)
	}
	if ops := openOperations(t, a); len(ops) != 1 {
		t.Fatalf("the unresolved operation must stay open: %+v", ops)
	}

	// The next batch recovers the finalized intent first and then updates
	// beta normally.
	a.deleteOperation = nil
	result, err = a.SyncSkills(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySlug = map[string]SyncItemResult{}
	for _, it := range result.Items {
		bySlug[it.Slug] = it
	}
	if bySlug["alpha"].Result != sync.ResultNoOp || bySlug["beta"].Result != sync.ResultUpdated {
		t.Fatalf("recovered batch: alpha %+v beta %+v", bySlug["alpha"], bySlug["beta"])
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, "beta", "notes.md")); err != nil || string(data) != "beta v2\n" {
		t.Fatalf("beta content after recovery: %q, %v", data, err)
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("recovery must clear the open operation: %+v", ops)
	}
}

// TestSyncDiffThreeWay locks the diff surface: materialized Source,
// live Store, and internal Baseline snapshots plus the three comparisons
// and path filtering.
func TestSyncDiffThreeWay(t *testing.T) {
	a := newTestApp(t)
	src, skillID := importOneSkill(t, a, "skills/demo")
	rewriteSourceFile(t, src, "skills/demo", "notes.md", "upstream\n")
	rewriteStoreFile(t, a, "demo", "local.txt", "local\n")

	r, err := a.DiffSkill(context.Background(), skillID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Comparisons) != 3 {
		t.Fatalf("comparisons: %d", len(r.Comparisons))
	}
	if r.Comparisons[0].From != "baseline" || r.Comparisons[0].To != "source" ||
		r.Comparisons[1].To != "store" || r.Comparisons[2].From != "source" {
		t.Fatalf("comparison order: %+v", r.Comparisons)
	}
	// baseline→source: notes.md added (content from the source side).
	if !hasEntry(r.Comparisons[0], "notes.md", sync.ChangeAdd) {
		t.Fatalf("baseline→source missing the added file: %+v", r.Comparisons[0].Entries)
	}
	// baseline→store: local.txt added.
	if !hasEntry(r.Comparisons[1], "local.txt", sync.ChangeAdd) {
		t.Fatalf("baseline→store missing the added file: %+v", r.Comparisons[1].Entries)
	}
	// Path filter narrows every comparison.
	filtered, err := a.DiffSkill(context.Background(), skillID, "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range filtered.Comparisons {
		for _, e := range c.Entries {
			if e.Path != "notes.md" {
				t.Fatalf("unfiltered entry %q", e.Path)
			}
		}
	}
	if !hasEntry(filtered.Comparisons[0], "notes.md", sync.ChangeAdd) {
		t.Fatalf("filtered comparison lost the entry: %+v", filtered.Comparisons[0].Entries)
	}
}

func hasEntry(c sync.Comparison, path, kind string) bool {
	for _, e := range c.Entries {
		if e.Path == path {
			for _, k := range e.Changes {
				if k == kind {
					return true
				}
			}
		}
	}
	return false
}
