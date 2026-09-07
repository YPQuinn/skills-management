package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/skillstore"
)

// strPtr returns a pointer to s for explicit slug overrides.
func strPtr(s string) *string { return &s }

// TestImportSkillsAbortedImportSurvivesDeleteFailure proves the terminal
// pre-install abort: an unmanaged live collision refuses before any
// mutation, the intent is durably marked aborted before its staging is
// discarded, and a failed intent deletion leaves a retryable aborted row.
// The next batch recovers the aborted row (preserving the unmanaged
// content) and imports a sibling.
func TestImportSkillsAbortedImportSurvivesDeleteFailure(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha", "skills/beta": "Beta"})
	// unmanaged content occupies the alpha slug
	unmanaged := filepath.Join(a.StorePath, "alpha")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res := importSkills(t, a, src.ID, "skills/alpha")
	item := res.Items[0]
	if item.Status != StatusFailed || item.ErrorCode != CodeRecovery {
		t.Fatalf("aborted item: %+v", item)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseAborted {
		t.Fatalf("aborted intent must remain: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(unmanaged, "mine.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("unmanaged content must be preserved: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(ops[0].ID))); !os.IsNotExist(err) {
		t.Fatal("staging must be discarded even when the intent deletion fails")
	}

	// the next batch recovers the aborted intent and imports the sibling
	a.deleteOperation = nil
	again := importSkills(t, a, src.ID, "skills/beta")
	if again.Items[0].Status != StatusImported {
		t.Fatalf("import after aborted recovery: %+v", again.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("aborted intent must be cleared after recovery: %+v", ops)
	}
}

// TestImportSkillsAbortedReplaceSurvivesDeleteFailure proves the same
// terminal abort for a Replace refused with ErrMissing (the managed live
// tree disappeared): the aborted row is retryable and the next batch
// recovers it before processing items.
func TestImportSkillsAbortedReplaceSurvivesDeleteFailure(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/one": "Alpha", "skills/two": "Beta"})
	first := importSkills(t, a, src.ID, "skills/one")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("initial import: %+v", first.Items[0])
	}
	// the managed live tree disappears; an explicit Replace then refuses
	// with ErrMissing before any mutation
	if err := os.RemoveAll(filepath.Join(a.StorePath, "alpha")); err != nil {
		t.Fatal(err)
	}
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res, err := a.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: src.ID,
		Selectors: []ImportSelector{{
			RelativeDir: "skills/two", Slug: strPtr("alpha"), Replace: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := res.Items[0]
	if item.Status != StatusFailed || item.ErrorCode != CodeRecovery || item.Slug != "alpha" {
		t.Fatalf("aborted replace item: %+v", item)
	}
	ops := openOperations(t, a)
	if len(ops) != 1 || ops[0].Phase != skillstore.PhaseAborted || ops[0].Kind != skillstore.KindReplace {
		t.Fatalf("aborted replace intent must remain: %+v", ops)
	}

	// the next batch recovers the aborted replace and the original entry is
	// again a no-op
	a.deleteOperation = nil
	again := importSkills(t, a, src.ID, "skills/one")
	if again.Items[0].Status != StatusAlreadyImported {
		t.Fatalf("import after aborted replace recovery: %+v", again.Items[0])
	}
	if ops := openOperations(t, a); len(ops) != 0 {
		t.Fatalf("aborted replace intent must be cleared: %+v", ops)
	}
}

// TestImportSkillsBlockedSiblingKeepsRequestedSlug proves a selection that
// follows an unresolved-intent item still reports its known requested slug
// (an explicit override) on the blocked outcome.
func TestImportSkillsBlockedSiblingKeepsRequestedSlug(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/a": "A", "skills/b": "B"})
	ctx, cancel := context.WithCancel(context.Background())
	a.commitHook = func(afterCommit bool) {
		if !afterCommit {
			cancel()
		}
	}
	a.deleteOperation = func(int64) error { return errors.New("delete failed") }

	res, err := a.ImportSkills(ctx, ImportSkillsInput{
		SourceID: src.ID,
		Selectors: []ImportSelector{
			{RelativeDir: "skills/a"},
			{RelativeDir: "skills/b", Slug: strPtr("b-explicit")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Status != StatusFailed || res.Items[0].ErrorCode != CodeRecovery {
		t.Fatalf("item leaving the unresolved intent: %+v", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed || res.Items[1].ErrorCode != CodeRecovery ||
		res.Items[1].RequestedSlug != "b-explicit" {
		t.Fatalf("blocked sibling must keep its requested slug: %+v", res.Items[1])
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, "b")); !os.IsNotExist(err) {
		t.Fatal("blocked sibling must not be installed")
	}
}
