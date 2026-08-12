package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestInstallSwapStagedAfterHashIsAmbiguous is the staged-tree TOCTOU
// regression: a byte-identical foreign tree is swapped into the staged slot
// after the staged tree was hashed from its opened handle. The source-name
// re-proof must refuse to install the replacement and report ErrAmbiguous
// with the foreign tree preserved.
func TestInstallSwapStagedAfterHashIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	proof, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	staged := s.stagedTreeDir(op.ID)
	s.hook = func(p HookPoint) {
		if p != HookAfterStagedHash {
			return
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, staged); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(staged); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, staged); err != nil {
			t.Fatal(err)
		}
	}

	err = s.Install(context.Background(), op, &proof)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("staged swap after hash: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(staged); err != nil {
		t.Fatal("the swapped staged tree must be preserved")
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may be installed from swapped content")
	}
}

// TestInstallSwapOldLiveAfterHashIsAmbiguous is the old-live TOCTOU
// regression: a byte-identical foreign tree is swapped into the live slot
// after the old live tree was hashed from its opened handle. The source-name
// re-proof must refuse to move the replacement into recovery and report
// ErrAmbiguous with the foreign live tree preserved.
func TestInstallSwapOldLiveAfterHashIsAmbiguous(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	proof, err := s.Stage(context.Background(), op.ID, replacement.dir, replacement.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	live := liveDir(t, s, "alpha")
	s.hook = func(p HookPoint) {
		if p != HookAfterOldLiveHash {
			return
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, live); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(live); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, live); err != nil {
			t.Fatal(err)
		}
	}

	err = s.Install(context.Background(), op, &proof)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("old-live swap after hash: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(live, "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the foreign live tree must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.recoveryDir(2)); !os.IsNotExist(err) {
		t.Fatal("no recovery slot may be created from swapped content")
	}
	if _, err := os.Lstat(s.stagedTreeDir(2)); err != nil {
		t.Fatal("the staged tree must be preserved")
	}
}

// TestFinalizeLayoutSwappedAfterOpenIsAmbiguous proves a public Store
// operation never reports success through detached handles: after the
// layout handles are pinned, the baselines logical path is detached and
// replaced with a fresh directory, so Finalize must return ErrAmbiguous
// without mutating the pinned baseline or clearing the operation.
func TestFinalizeLayoutSwappedAfterOpenIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	detached := filepath.Join(s.InternalDir(), "baselines.real")
	s.hook = func(p HookPoint) {
		if p != HookAfterLayoutOpen {
			return
		}
		// detach baselines: the logical name now names a fresh directory
		if err := os.Rename(filepath.Join(s.InternalDir(), "baselines"), detached); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(s.InternalDir(), "baselines"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("detached layout: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(detached, "7")); !os.IsNotExist(err) {
		t.Fatal("finalize must not mutate through a detached handle")
	}
	if _, err := os.Lstat(s.baselineCandidateDir(1)); err != nil {
		t.Fatal("operation candidates must be preserved")
	}
	if _, err := os.Lstat(filepath.Join(s.opDir(1), "proof")); err != nil {
		t.Fatal("operation evidence must be preserved")
	}
}

// TestFinalizeBaselineSwapBeforeMoveIsAmbiguous proves the Baseline source
// move re-proves the baseline slot by physical identity: for a replace, the
// active baseline is replaced with a byte-identical foreign copy after it
// was sampled but before the first move, so finalization must refuse on the
// identity compare alone — a digest-only check would let the foreign copy
// be rotated in — and preserve both the original baseline and the operation
// candidates.
func TestFinalizeBaselineSwapBeforeMoveIsAmbiguous(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	op.SkillID = 7
	saved := filepath.Join(s.InternalDir(), "baseline7.real")
	s.hook = func(p HookPoint) {
		if p != HookBeforeBaselineMoves {
			return
		}
		if err := os.Rename(filepath.Join(s.InternalDir(), "baselines", "7"), saved); err != nil {
			t.Fatal(err)
		}
		swap := t.TempDir()
		if err := copyTreeForTest(swap, saved); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, filepath.Join(s.InternalDir(), "baselines", "7")); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("baseline swap before move: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(filepath.Join(saved, "SKILL.md")); err != nil {
		t.Fatal("the original baseline must be preserved")
	}
	if data, rerr := os.ReadFile(filepath.Join(s.InternalDir(), "baselines", "7", "SKILL.md")); rerr != nil || string(data) != "old content" {
		t.Fatalf("the byte-identical foreign baseline must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(filepath.Join(s.recoveryDir(2), "SKILL.md")); err != nil {
		t.Fatal("the recovery candidate must be preserved")
	}
	if _, err := os.Lstat(s.baselineCandidateDir(2)); err != nil {
		t.Fatal("operation candidates must be preserved")
	}
}
