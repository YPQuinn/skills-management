package skillstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestStageSwapTreeAfterHashFailsPreservesForeign is the Stage TOCTOU
// regression: a byte-identical foreign tree is swapped into the staged
// tree slot after the staged tree was hashed from its opened handle. The
// post-hash re-proofs must refuse Stage's success and preserve the foreign
// tree, so Install can never be handed a proof for content Stage did not
// verify.
func TestStageSwapTreeAfterHashFailsPreservesForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	s.hook = func(p HookPoint) {
		if p != HookAfterStageHash {
			return
		}
		tree := s.stagedTreeDir(1)
		swap := t.TempDir()
		if err := copyTreeForTest(swap, tree); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(tree); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, tree); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("staged-tree swap after hash: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.stagedTreeDir(1)); err != nil {
		t.Fatal("the swapped foreign staged tree must be preserved")
	}
}

// TestStageSwapOpDirAfterHashFailsPreservesForeign is the operation-directory
// TOCTOU regression: the whole operation directory is swapped for a
// byte-identical foreign directory after the staged tree was hashed. Stage
// must refuse and preserve the foreign operation directory, so a retry or
// recovery never mistakes it for the operation's own staging.
func TestStageSwapOpDirAfterHashFailsPreservesForeign(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	s.hook = func(p HookPoint) {
		if p != HookAfterStageHash {
			return
		}
		opDir := s.opDir(1)
		swap := t.TempDir()
		if err := copyTreeForTest(swap, opDir); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(opDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(swap, opDir); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Stage(context.Background(), 1, m.dir, m.digest, false)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("operation-directory swap after hash: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.opDir(1)); err != nil {
		t.Fatal("the swapped foreign operation directory must be preserved")
	}
}

// TestInstallRejectsForeignStagedTree proves Install binds the staged tree
// to the exact object Stage proved: a byte-identical foreign tree swapped
// in after Stage returned its proof is refused by physical identity even
// though its digest matches, and is preserved with no live mutation.
func TestInstallRejectsForeignStagedTree(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	proof, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	// swap the staged tree for a byte-identical foreign directory after
	// Stage proved the original
	tree := s.stagedTreeDir(op.ID)
	swap := t.TempDir()
	if err := copyTreeForTest(swap, tree); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(tree); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(swap, tree); err != nil {
		t.Fatal(err)
	}

	err = s.Install(context.Background(), op, &proof)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign staged tree: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(tree); err != nil {
		t.Fatal("the foreign staged tree must be preserved")
	}
	if _, err := os.Lstat(liveDir(t, s, "alpha")); !os.IsNotExist(err) {
		t.Fatal("no live tree may be installed from a foreign staged tree")
	}
}

// TestRemoveOpRefusesForeignOpDir proves operation cleanup drains only the
// operation directory that was opened and verified: when the logical entry
// is swapped for a foreign non-empty directory before the drain, cleanup
// refuses with ErrAmbiguous and the foreign content is preserved. The
// pre-install discard runs with the identity Stage retained.
func TestRemoveOpRefusesForeignOpDir(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	staged, err := s.Stage(context.Background(), op.ID, m.dir, m.digest, false)
	if err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookBeforeRemoveOp {
			return
		}
		opDir := s.opDir(op.ID)
		if err := os.Rename(opDir, opDir+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(opDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(opDir, "foreign.txt"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err = s.DiscardStaging(op.ID, staged)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("foreign operation directory: got %v, want ErrAmbiguous", err)
	}
	if data, rerr := os.ReadFile(filepath.Join(s.opDir(op.ID), "foreign.txt")); rerr != nil || string(data) != "mine" {
		t.Fatalf("foreign non-empty content must be preserved: %q, %v", data, rerr)
	}
	if _, err := os.Lstat(s.opDir(op.ID) + ".real"); err != nil {
		t.Fatal("the real operation directory must survive")
	}
}

// TestEnsureLayoutRootSwapRejected proves EnsureLayout re-proves the Store
// root before returning success: a root swapped for a fresh directory after
// the internal layout was created is refused, and the real layout survives.
func TestEnsureLayoutRootSwapRejected(t *testing.T) {
	s := newStore(t)
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	s.hook = func(p HookPoint) {
		if p != HookBeforeEnsureLayoutVerify {
			return
		}
		if err := os.Rename(s.Root, s.Root+".real"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(s.Root, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.EnsureLayout(); err == nil {
		t.Fatal("a swapped Store root must be refused")
	}
	if _, err := os.Lstat(filepath.Join(s.Root+".real", ".skillctl", "staging")); err != nil {
		t.Fatal("the real layout must survive at the original root")
	}
	if _, err := os.Lstat(filepath.Join(s.Root, ".skillctl")); !os.IsNotExist(err) {
		t.Fatal("nothing may be blessed inside the swapped root")
	}
}

// TestFinalizeStagingReappearsAfterDrainIsAmbiguous proves Finalize never
// authorizes clearing the journal when the operation entry reappears after
// its drain: the reappearance is foreign activity and is reported as
// ErrAmbiguous with the journal preserved.
func TestFinalizeStagingReappearsAfterDrainIsAmbiguous(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	op := Operation{ID: 1, Slug: "alpha", Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = 7
	s.hook = func(p HookPoint) {
		if p != HookAfterRemoveOp {
			return
		}
		if err := os.Mkdir(s.opDir(op.ID), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("reappeared operation entry after drain: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.opDir(op.ID)); err != nil {
		t.Fatal("the reappeared entry must be preserved")
	}
}

// TestRotatePreviousDuplicateRecoveryIsAmbiguous proves a byte-identical
// recovery slot alongside the installed previous snapshot is contradictory
// duplicate evidence: rotation must preserve both and report ErrAmbiguous
// instead of deleting either copy.
func TestRotatePreviousDuplicateRecoveryIsAmbiguous(t *testing.T) {
	s := newStore(t)
	old := buildMaterialized(t, map[string]string{"SKILL.md": "old content"})
	importAndFinalize(t, s, 1, "alpha", old, 7)

	replacement := buildMaterialized(t, map[string]string{"SKILL.md": "new content"})
	op := Operation{ID: 2, SkillID: 7, Slug: "alpha", Kind: KindReplace, OldDigest: old.digest, NewDigest: replacement.digest}
	stageAndInstall(t, s, op, replacement)
	// simulate the crash state where rotation completed (recovery content
	// is the previous snapshot) and a byte-identical duplicate recovery
	// slot reappeared
	if err := os.Rename(s.recoveryDir(2), s.PreviousDir(7)); err != nil {
		t.Fatal(err)
	}
	dup := t.TempDir()
	if err := copyTreeForTest(dup, s.PreviousDir(7)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dup, s.recoveryDir(2)); err != nil {
		t.Fatal(err)
	}

	err := s.Finalize(context.Background(), op)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("duplicate previous/recovery evidence: got %v, want ErrAmbiguous", err)
	}
	if _, err := os.Lstat(s.PreviousDir(7)); err != nil {
		t.Fatal("the installed previous snapshot must be preserved")
	}
	if _, err := os.Lstat(s.recoveryDir(2)); err != nil {
		t.Fatal("the duplicate recovery slot must be preserved")
	}
}
