package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestAbortRefusesPresentStagingPreservesLive proves recovery only ever
// sees the absent staging of an aborted pre-install refusal: a staging
// directory that still exists has no persisted physical proof on restart
// (Stage-A writes no durable receipt), so Abort preserves it and the
// unmanaged live content with ErrAmbiguous instead of draining a fresh
// sample. The immediate abort path discards with DiscardStaging and the
// identity Stage retained; recovery then clears the absent state.
func TestAbortRefusesPresentStagingPreservesLive(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "mine.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Abort(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("abort with present staging: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("staging must be preserved")
	}
	if data, err := os.ReadFile(filepath.Join(live, "mine.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("unmanaged live content must be preserved: %q, %v", data, err)
	}
}

// TestAbortRefusesPartialLayoutRecoverySlot proves an abort never clears
// evidence through a partial layout: a recovery slot can exist even when
// the rest of the internal layout is missing, and it is preserved with
// ErrAmbiguous instead of being treated as absence.
func TestAbortRefusesPartialLayoutRecoverySlot(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "d"}
	// a recovery slot with no staging and no other layout component
	rec := s.recoveryDir(1)
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rec, "old"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Abort(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial-layout recovery slot: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(rec); err != nil {
		t.Fatal("the recovery slot must be preserved")
	}
}

// TestAbortWithoutAnyLayoutSucceeds proves the never-initialized state is
// still abortable: no Store root means no operation artifact can exist.
func TestAbortWithoutAnyLayoutSucceeds(t *testing.T) {
	s := Store{Root: filepath.Join(t.TempDir(), "absent")}
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "d"}
	if err := s.Abort(context.Background(), op); err != nil {
		t.Fatalf("abort with no Store at all: %v", err)
	}
}

// TestAbortRefusesTerminalTombstone proves the deterministic terminal
// tombstone is contradictory evidence for a pre-install abort: any object
// at staging/.skillctl-term-<opID>, regardless of bytes or type, is
// preserved with ErrAmbiguous.
func TestAbortRefusesTerminalTombstone(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "d"}
	tomb := filepath.Join(s.InternalDir(), "staging", terminalSlotName(1))
	if err := os.MkdirAll(tomb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tomb, "leftover"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Abort(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("terminal tombstone: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(tomb); err != nil {
		t.Fatal("the terminal tombstone must be preserved")
	}
}

// TestAbortRefusesTerminalTombstonePartialLayout proves the same refusal
// through the partial-layout path: a staging directory holding only the
// terminal tombstone is never treated as absence.
func TestAbortRefusesTerminalTombstonePartialLayout(t *testing.T) {
	s := newStore(t)
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: "d"}
	// only staging with the tombstone exists; the rest of the layout is
	// missing so the full layout cannot open
	tomb := filepath.Join(s.InternalDir(), "staging", terminalSlotName(1))
	if err := os.MkdirAll(tomb, 0o755); err != nil {
		t.Fatal(err)
	}

	err := s.Abort(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("partial-layout terminal tombstone: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(tomb); err != nil {
		t.Fatal("the terminal tombstone must be preserved")
	}
}

// TestAbortIsIdempotent proves an aborted operation whose staging was
// already provably discarded (by the immediate abort path) still completes,
// so a failed intent deletion can be retried on the next write.
func TestAbortIsIdempotent(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DiscardStaging(op.ID, staged); err != nil {
		t.Fatal(err)
	}
	if err := s.Abort(context.Background(), op); err != nil {
		t.Fatalf("abort after completion must be idempotent: %v", err)
	}
}

// TestAbortRejectsContradictoryEvidence proves a pre-install abort can
// never have created a recovery slot; its presence is preserved and
// reported as ErrAmbiguous rather than discarded.
func TestAbortRejectsContradictoryEvidence(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	if _, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.recoveryDir(1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.recoveryDir(1), "old"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := s.Abort(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("contradictory abort evidence: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.recoveryDir(1)); err != nil {
		t.Fatal("contradictory evidence must be preserved")
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("operation staging must be preserved")
	}
}
