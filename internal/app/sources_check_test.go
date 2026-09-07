package app

import (
	"context"
	"os"
	"testing"

	"skillctl/internal/source"
)

func TestCheckSourceLifecycle(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root, Name: "life"})
	if err != nil {
		t.Fatal(err)
	}

	// a second Skill appears; check replaces the whole Inventory
	writeSourceSkill(t, root, "beta")
	checked, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !checked.Available || len(checked.Entries) != 2 {
		t.Fatalf("after check: %+v", checked.Entries)
	}

	// the Source becomes unavailable: check keeps the Inventory and records
	// the failure, advancing the check timestamp beyond the last successful
	// check while preserving the last success and the retained Inventory
	lastOK := checked.LastCheckedAt
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	checked, err = a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.Available || checked.LastError == "" {
		t.Fatalf("unavailable check: %+v", checked)
	}
	if checked.LastCheckedAt == nil || lastOK == nil || !checked.LastCheckedAt.After(*lastOK) {
		t.Fatalf("failed check must advance LastCheckedAt beyond the last successful check: %v -> %v", lastOK, checked.LastCheckedAt)
	}
	if checked.LastSuccessfulCheckAt == nil || !checked.LastSuccessfulCheckAt.Equal(*lastOK) {
		t.Fatalf("failed check must preserve LastSuccessfulCheckAt exactly: %+v", checked.LastSuccessfulCheckAt)
	}
	if len(checked.Entries) != 2 {
		t.Fatalf("stale Inventory must be retained: %+v", checked.Entries)
	}

	// list shows staleness
	items, err := a.ListSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Stale || items[0].EntryCount != 2 {
		t.Fatalf("summary: %+v", items[0])
	}

	// show still returns the retained Inventory
	shown, err := a.ShowSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shown.Entries) != 2 || shown.Issues != nil {
		t.Fatalf("show: %+v", shown)
	}

	// the Source comes back: check recovers
	if err := os.Rename(root+"-moved", root); err != nil {
		t.Fatal(err)
	}
	checked, err = a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !checked.Available || checked.LastError != "" {
		t.Fatalf("recovered check: %+v", checked)
	}

	// unknown ids
	if _, err := a.CheckSource(context.Background(), 4242); !isCode(err, CodeNotFound) {
		t.Fatalf("check unknown: %v", err)
	}
	if _, err := a.ShowSource(4242); !isCode(err, CodeNotFound) {
		t.Fatalf("show unknown: %v", err)
	}
}

func TestCheckTimestampsAdvanceWithinSameSecond(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}

	// two consecutive checks run within the same wall-clock second; the
	// stored timestamps must still advance thanks to fractional precision
	first, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.LastCheckedAt == nil || second.LastCheckedAt == nil {
		t.Fatal("check timestamps missing")
	}
	if !second.LastCheckedAt.After(*first.LastCheckedAt) {
		t.Fatalf("consecutive checks must advance LastCheckedAt with fractional precision: %v -> %v",
			first.LastCheckedAt, second.LastCheckedAt)
	}
	if first.LastCheckStartedAt == nil || second.LastCheckStartedAt == nil || !second.LastCheckStartedAt.After(*first.LastCheckStartedAt) {
		t.Fatalf("consecutive checks must advance LastCheckStartedAt: %v -> %v",
			first.LastCheckStartedAt, second.LastCheckStartedAt)
	}
}

func TestCheckMetadataLifecycle(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	root := t.TempDir()
	writeSourceSkill(t, root, "alpha")
	src, err := a.AddSource(context.Background(), source.AddInput{Kind: source.KindLocal, Location: root})
	if err != nil {
		t.Fatal(err)
	}

	// a failed check records failed metadata but keeps commit, digest, and
	// Inventory
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	failed, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.LastCheckResult != source.CheckResultFailed || failed.Available {
		t.Fatalf("failed check metadata: %+v", failed)
	}
	if failed.LastInventoryDigest != src.LastInventoryDigest || failed.LastCommit != src.LastCommit {
		t.Fatalf("failed check must retain digest and commit: %+v (was %+v)", failed, src)
	}

	// a successful check records ok metadata and a fresh digest
	if err := os.Rename(root+"-moved", root); err != nil {
		t.Fatal(err)
	}
	writeSourceSkill(t, root, "beta")
	recovered, err := a.CheckSource(context.Background(), src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.LastCheckResult != source.CheckResultOK || !recovered.Available || recovered.LastError != "" {
		t.Fatalf("recovered check metadata: %+v", recovered)
	}
	if recovered.LastInventoryDigest == "" || len(recovered.Entries) != 2 {
		t.Fatalf("recovered check must replace the Inventory: %+v", recovered)
	}
	for _, e := range recovered.Entries {
		if e.Digest == "" {
			t.Fatalf("entry digest missing: %+v", e)
		}
	}
}
