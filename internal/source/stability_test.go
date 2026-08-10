package source

import (
	"context"
	"errors"
	"testing"
	"time"
)

func snapOf(rel ...string) snapshot {
	var s snapshot
	for _, r := range rel {
		s.entries = append(s.entries, snapshotEntry{relPath: r, kind: kindDir})
	}
	return s
}

func TestObserveStableAcceptsAgreeingScans(t *testing.T) {
	calls := 0
	obs, err := observeStable(context.Background(), time.Millisecond, func(context.Context) (localScan, error) {
		calls++
		return localScan{obs: Observation{Entries: []Entry{{RelativeDir: "skills/a"}}}, snap: snapOf("skills/a")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("scans: got %d, want 2", calls)
	}
	if len(obs.Entries) != 1 || obs.Entries[0].RelativeDir != "skills/a" {
		t.Fatalf("observation: %+v", obs)
	}
}

func TestObserveStableRetriesThenAccepts(t *testing.T) {
	scans := []localScan{
		{obs: Observation{Entries: []Entry{{RelativeDir: "skills/a"}}}, snap: snapOf("skills/a")},
		{obs: Observation{Entries: []Entry{{RelativeDir: "skills/a"}, {RelativeDir: "skills/b"}}}, snap: snapOf("skills/a", "skills/b")},
		{obs: Observation{Entries: []Entry{{RelativeDir: "skills/a"}, {RelativeDir: "skills/b"}}}, snap: snapOf("skills/a", "skills/b")},
	}
	calls := 0
	obs, err := observeStable(context.Background(), time.Millisecond, func(context.Context) (localScan, error) {
		if calls >= len(scans) {
			t.Fatalf("scan called %d times, want at most %d", calls+1, len(scans))
		}
		s := scans[calls]
		calls++
		return s, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("scans: got %d, want 3 (one retry)", calls)
	}
	if len(obs.Entries) != 2 {
		t.Fatalf("observation must be the settled scan: %+v", obs.Entries)
	}
}

func TestObserveStableFailsOnContinuedChange(t *testing.T) {
	scans := []localScan{
		{snap: snapOf("skills/a")},
		{snap: snapOf("skills/a", "skills/b")},
		{snap: snapOf("skills/a", "skills/b", "skills/c")},
	}
	calls := 0
	_, err := observeStable(context.Background(), time.Millisecond, func(context.Context) (localScan, error) {
		s := scans[calls]
		calls++
		return s, nil
	})
	if err == nil {
		t.Fatal("continued change: want error")
	}
	if calls != 3 {
		t.Fatalf("scans: got %d, want 3", calls)
	}
}

func TestObserveStableHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := observeStable(ctx, time.Hour, func(context.Context) (localScan, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return localScan{snap: snapOf("skills/a")}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: got %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("scans: got %d, want 1 (cancellation between scans)", calls)
	}
}
