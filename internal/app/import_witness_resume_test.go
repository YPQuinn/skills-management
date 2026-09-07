package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// statID returns the physical identity of one path for receipt assertions.
func statID(t *testing.T, path string) fileIDView {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	return fileIDView{dev: uint64(st.Dev), ino: uint64(st.Ino)}
}

type fileIDView struct {
	dev uint64
	ino uint64
}

// TestImportSkillsFreshAppResumesStalePreviousWitnessWithStableReceipt
// proves the stale-previous witness window at the application boundary: a
// fresh App resumes the interrupted rotation through the witness-bound
// identity, and the durable receipt it persists is byte-identical to the
// canonical receipt the terminal state requires (opDir/proof captured
// before recovery; live, Baseline, and previous bound to the actual
// terminal objects).
func TestImportSkillsFreshAppResumesStalePreviousWitnessWithStableReceipt(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	// replace #1 finalizes the first previous snapshot
	srcB := addReplacementSource(t, a)
	op1 := replaceAndCommit(t, a, srcB, "skills/alpha", "alpha")
	receipt1, err := a.store.PrepareFinalize(context.Background(), op1)
	if err != nil {
		t.Fatal(err)
	}
	op1.Phase = skillstore.PhaseFinalized
	op1.Receipt = receipt1.Bytes()
	if err := state.MarkOperationFinalized(a.db, op1); err != nil {
		t.Fatal(err)
	}
	if err := a.store.CleanupTerminal(context.Background(), op1, receipt1); err != nil {
		t.Fatal(err)
	}
	if err := state.DeleteOperationTerminal(a.db, op1); err != nil {
		t.Fatal(err)
	}
	// replace #2 is committed but its finalization crashes between the
	// stale-snapshot move and the recovery rotation
	rootC := t.TempDir()
	writeSkillFile(t, rootC, "skills/alpha", "Alpha")
	if err := os.WriteFile(filepath.Join(rootC, "skills", "alpha", "SKILL.md"), []byte(v2Markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	srcC, err := a.AddSource(context.Background(), sourceAddInput(rootC))
	if err != nil {
		t.Fatal(err)
	}
	op2 := replaceAndCommit(t, a, srcC, "skills/alpha", "alpha")
	detail, err := state.GetSkillDetailBySlug(a.db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	prevName := filepath.Join(a.StorePath, ".skillctl", "previous", itoa(detail.Skill.ID))
	staleName := prevName + ".old"
	if err := os.Rename(prevName, staleName); err != nil {
		t.Fatal(err)
	}
	stInfo, err := os.Lstat(staleName)
	if err != nil {
		t.Fatal(err)
	}
	stSt := stInfo.Sys().(*syscall.Stat_t)
	stRoot, err := os.OpenRoot(staleName)
	if err != nil {
		t.Fatal(err)
	}
	staleDigest, err := source.TreeDigestRoot(context.Background(), stRoot)
	stRoot.Close()
	if err != nil {
		t.Fatal(err)
	}
	witnessPath := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op2.ID), "stale-previous-witness")
	if err := os.WriteFile(witnessPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	wInfo, err := os.Lstat(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	wSt := wInfo.Sys().(*syscall.Stat_t)
	witness := fmt.Sprintf("1\n%d\nstale-previous\n%d %d\n%d %d\n%s\n",
		op2.ID, wSt.Dev, wSt.Ino, stSt.Dev, stSt.Ino, staleDigest)
	if err := os.WriteFile(witnessPath, []byte(witness), 0o644); err != nil {
		t.Fatal(err)
	}
	// capture the evidence identities the receipt must bind
	opDirID := statID(t, filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op2.ID)))
	proofID := statID(t, filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op2.ID), "proof"))

	fresh := reopenAppUnrecovered(t, a)
	fresh.deleteOperation = func(int64) error { return errors.New("delete failed") }
	if _, err := fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcC.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	}); !isCode(err, CodeRecovery) {
		t.Fatalf("the terminal delete failure must block: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op2.ID || ops[0].Phase != skillstore.PhaseFinalized || len(ops[0].Receipt) == 0 {
		t.Fatalf("the resumed replace must persist its receipt: %+v", ops)
	}
	// the persisted receipt is byte-identical to the canonical receipt of
	// the terminal state
	liveID := statID(t, filepath.Join(a.StorePath, "alpha"))
	baseID := statID(t, filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op2.SkillID)))
	prevID := statID(t, prevName)
	expected := fmt.Sprintf("1\nfinalize\n%d\n%d\nalpha\nreplace\n%s\n%s\n%d %d\n%d %d\n%d %d\n%d %d\n%d %d\n",
		op2.ID, op2.SkillID, op2.OldDigest, op2.NewDigest,
		opDirID.dev, opDirID.ino, proofID.dev, proofID.ino,
		liveID.dev, liveID.ino, baseID.dev, baseID.ino, prevID.dev, prevID.ino)
	if string(ops[0].Receipt) != expected {
		t.Fatalf("the resumed receipt is not byte-stable:\n%q\nwant\n%q", ops[0].Receipt, expected)
	}
	// a fresh App without the failure converges
	converged := reopenApp(t, fresh)
	again := importSkills(t, converged, srcC.ID, "skills/alpha")
	if again.Items[0].Status != StatusAlreadyImported || again.Items[0].SkillID != op2.SkillID {
		t.Fatalf("item after convergence: %+v", again.Items[0])
	}
	if ops := openOperations(t, converged); len(ops) != 0 {
		t.Fatalf("the replace terminal row must be cleared: %+v", ops)
	}
}

// TestImportSkillsFreshAppBlocksCopiedWitness proves the witness
// self-identity binding at the application boundary: a byte-identical
// witness copied into a fresh inode is refused by fresh-App recovery with
// CodeRecovery, the committed row is kept, and the foreign witness and the
// witnessed backup are preserved.
func TestImportSkillsFreshAppBlocksCopiedWitness(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	if first.Items[0].Status != StatusImported {
		t.Fatalf("setup import: %+v", first.Items[0])
	}
	srcB := addReplacementSource(t, a)
	op := replaceAndCommit(t, a, srcB, "skills/alpha", "alpha")
	// reproduce the crash state between the Baseline backup move and the
	// candidate move (canonical witness), then copy the witness into a
	// fresh inode
	base := filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(op.SkillID))
	backup := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline-old")
	if err := os.Rename(base, backup); err != nil {
		t.Fatal(err)
	}
	bInfo, err := os.Lstat(backup)
	if err != nil {
		t.Fatal(err)
	}
	bSt := bInfo.Sys().(*syscall.Stat_t)
	bRoot, err := os.OpenRoot(backup)
	if err != nil {
		t.Fatal(err)
	}
	baseDigest, err := source.TreeDigestRoot(context.Background(), bRoot)
	bRoot.Close()
	if err != nil {
		t.Fatal(err)
	}
	witnessPath := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline-backup-witness")
	if err := os.WriteFile(witnessPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	wInfo, err := os.Lstat(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	wSt := wInfo.Sys().(*syscall.Stat_t)
	witness := fmt.Sprintf("1\n%d\nbaseline-old\n%d %d\n%d %d\n%s\n",
		op.ID, wSt.Dev, wSt.Ino, bSt.Dev, bSt.Ino, baseDigest)
	if err := os.WriteFile(witnessPath, []byte(witness), 0o644); err != nil {
		t.Fatal(err)
	}
	// copy the witness into a fresh inode
	data, err := os.ReadFile(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	tmp := witnessPath + ".copy"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, witnessPath); err != nil {
		t.Fatal(err)
	}

	fresh := reopenAppUnrecovered(t, a)
	_, err = fresh.ImportSkills(context.Background(), ImportSkillsInput{
		SourceID: srcB.ID, Selectors: []ImportSelector{{RelativeDir: "skills/alpha"}},
	})
	if !isCode(err, CodeRecovery) {
		t.Fatalf("copied witness at the application seam: %v", err)
	}
	ops := openOperations(t, fresh)
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].Phase != skillstore.PhaseCommitted {
		t.Fatalf("the committed row must be kept: %+v", ops)
	}
	if _, err := os.Lstat(witnessPath); err != nil {
		t.Fatal("the copied witness must be preserved")
	}
	if _, err := os.Lstat(backup); err != nil {
		t.Fatal("the witnessed backup must be preserved")
	}
}
