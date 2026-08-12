package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// cleanupFinalized runs the terminal cleanup for a prepared import as a
// fresh process would: the receipt is parsed from the durable bytes and the
// operation carries the terminal phase.
func cleanupFinalized(t *testing.T, s Store, op Operation, receipt CleanupReceipt) error {
	t.Helper()
	op.Phase = PhaseFinalized
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return s.CleanupTerminal(context.Background(), op, parsed)
}

func TestCleanupTerminalRemovesEvidenceAndIsIdempotent(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	if err := cleanupFinalized(t, s, op, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(op.ID)); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the operation directory")
	}
	if _, err := os.Lstat(filepath.Join(s.BaselineDir(op.SkillID), "SKILL.md")); err != nil {
		t.Fatal("cleanup must not touch the installed Baseline")
	}
	// a fresh Store instance cleaning the already-clean state converges
	if err := cleanupFinalized(t, New(s.Root), op, receipt); err != nil {
		t.Fatalf("idempotent cleanup: %v", err)
	}
}

func TestCleanupTerminalResumesWithoutProof(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	// crash after the proof was removed, before the operation directory
	if err := os.Remove(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal(err)
	}
	if err := cleanupFinalized(t, New(s.Root), op, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(op.ID)); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the empty operation directory")
	}
}

func TestCleanupTerminalResumesFromCapturedProofSlot(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	// crash after the proof was detached into its deterministic slot inside
	// the operation directory, before the slot was unlinked
	if err := os.Rename(filepath.Join(s.opDir(op.ID), "proof"),
		filepath.Join(s.opDir(op.ID), proofSlotName(op.ID))); err != nil {
		t.Fatal(err)
	}
	if err := cleanupFinalized(t, New(s.Root), op, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(op.ID)); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the operation directory and its captured slot")
	}
}

func TestCleanupTerminalResumesFromTerminalSlot(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	// crash after the proof was removed and the empty operation directory
	// was detached into the deterministic terminal slot, before the slot
	// was unlinked
	if err := os.Remove(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(s.opDir(op.ID), filepath.Join(s.InternalDir(), "staging", terminalSlotName(op.ID))); err != nil {
		t.Fatal(err)
	}
	if err := cleanupFinalized(t, New(s.Root), op, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.InternalDir(), "staging", terminalSlotName(op.ID))); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the terminal slot")
	}
}

// TestCleanupTerminalRefusesForeignEvidence proves the receipt-bound
// evidence rules: a wrong operation-directory identity, an unexpected
// child, or a conflicting terminal slot is preserved with ErrAmbiguous and
// the row is never cleared.
func TestCleanupTerminalRefusesForeignEvidence(t *testing.T) {
	t.Run("foreign opDir", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		real := s.opDir(op.ID) + ".real"
		if err := os.Rename(s.opDir(op.ID), real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, s.opDir(op.ID)); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("foreign operation directory: got %v, want ErrAmbiguous", err)
		}
		if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
			t.Fatal("the foreign evidence must be preserved")
		}
	})
	t.Run("unexpected child", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		if err := os.WriteFile(filepath.Join(s.opDir(op.ID), "foreign.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("unexpected child: got %v, want ErrAmbiguous", err)
		}
		if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), "foreign.txt")); rerr != nil || string(data) != "mine" {
			t.Fatalf("the foreign child must be preserved: %q, %v", data, rerr)
		}
		if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
			t.Fatal("the proof must be preserved with the foreign child")
		}
	})
	t.Run("conflicting terminal slot", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		if err := os.MkdirAll(filepath.Join(s.InternalDir(), "staging", terminalSlotName(op.ID)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("conflicting terminal slot: got %v, want ErrAmbiguous", err)
		}
		if _, err := os.Lstat(s.opDir(op.ID)); err != nil {
			t.Fatal("the operation directory must be preserved")
		}
	})
}

func TestCleanupTerminalRequiresMatchingActionAndPhase(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	op.Phase = PhaseRestored
	if err := s.CleanupTerminal(context.Background(), op, receipt); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("restored row with a finalize receipt: got %v, want ErrAmbiguous", err)
	}
	op.Phase = PhasePending
	if err := s.CleanupTerminal(context.Background(), op, receipt); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("non-terminal row: got %v, want ErrAmbiguous", err)
	}
}

// TestCleanupTerminalRejectsMismatchedOperation proves the receipt is
// bound to the full operation identity: a stale row field refuses the
// cleanup before any evidence is touched.
func TestCleanupTerminalRejectsMismatchedOperation(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedImport(t, s)
	op.Phase = PhaseFinalized
	op.Slug = "beta"
	if err := s.CleanupTerminal(context.Background(), op, receipt); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("mismatched slug: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be preserved")
	}
}

func TestCleanupTerminalValidatesTerminalResult(t *testing.T) {
	t.Run("missing live", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		if err := os.RemoveAll(liveDir(t, s, "alpha")); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("missing live tree: got %v, want ErrAmbiguous", err)
		}
	})
	t.Run("byte-identical foreign live", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		live := liveDir(t, s, "alpha")
		real := live + ".real"
		if err := os.Rename(live, real); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, live); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("foreign live tree: got %v, want ErrAmbiguous", err)
		}
		if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
			t.Fatal("the evidence must be preserved with the foreign live tree")
		}
		if _, err := os.Lstat(filepath.Join(real, "SKILL.md")); err != nil {
			t.Fatal("the installed live tree must survive beside the foreign one")
		}
	})
	t.Run("corrupt baseline", func(t *testing.T) {
		s := newStore(t)
		op, receipt := preparedImport(t, s)
		if err := os.WriteFile(filepath.Join(s.BaselineDir(op.SkillID), "SKILL.md"), []byte("tampered"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := cleanupFinalized(t, New(s.Root), op, receipt); !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("corrupt Baseline: got %v, want ErrAmbiguous", err)
		}
	})
}

// TestCleanupTerminalRestoredImportRefusesLive proves a restored import's
// terminal expectation: any live tree present after an import restore is
// foreign and blocks the cleanup with the row preserved.
func TestCleanupTerminalRestoredImportRefusesLive(t *testing.T) {
	s := newStore(t)
	op, receipt := preparedRestoreImport(t, s)
	// a foreign live tree appears while the terminal row waits for cleanup
	live := liveDir(t, s, "alpha")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "SKILL.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	op.Phase = PhaseRestored
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(context.Background(), op, parsed); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("live after import restore: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(op.ID), "proof")); err != nil {
		t.Fatal("the evidence must be preserved")
	}
}

// TestCleanupTerminalRestoredReplaceValidatesLive proves a restored
// replace's terminal expectation: the live tree must be the exact
// receipt-bound recovered object with the old digest.
func TestCleanupTerminalRestoredReplaceValidatesLive(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)
	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	receipt, err := s.PrepareRestore(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	op.Phase = PhaseRestored
	parsed, err := ParseCleanupReceipt(receipt.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupTerminal(context.Background(), op, parsed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.opDir(op.ID)); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove the evidence of the restored replace")
	}
	if data, rerr := os.ReadFile(filepath.Join(liveDir(t, s, "alpha"), "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the restored live tree must survive: %q, %v", data, rerr)
	}
}
