package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
	"skillctl/internal/state"
)

// TestImportSkillsFreshAppResumesInterruptedReplaceFinalize proves the
// finalize crash window at the application boundary: a committed replace
// whose preparation crashed between the Baseline backup move and the
// candidate move (the moved-aside Baseline and its durable witness survive)
// is resumed by a fresh App through the witness-bound identity, the
// Baseline advance and previous rotation complete, the receipt persists,
// and the row is cleared.
func TestImportSkillsFreshAppResumesInterruptedReplaceFinalize(t *testing.T) {
	a := newTestApp(t)
	src := addLocalSource(t, a, map[string]string{"skills/alpha": "Alpha"})
	first := importSkills(t, a, src.ID, "skills/alpha")
	skillID := first.Items[0].SkillID
	rootB := t.TempDir()
	writeSkillFile(t, rootB, "skills/alpha", "Alpha")
	if err := os.WriteFile(filepath.Join(rootB, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcB, err := a.AddSource(context.Background(), sourceAddInput(rootB))
	if err != nil {
		t.Fatal(err)
	}
	dir, digest := materializeTo(t, a, srcB, "skills/alpha")
	detail, err := state.GetSkillDetailBySlug(a.db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	op := skillstore.Operation{
		SkillID: detail.Skill.ID, Slug: detail.Skill.Slug, Kind: skillstore.KindReplace,
		OldDigest: detail.Skill.StoreDigest, NewDigest: digest,
	}
	op, err = state.InsertOperation(a.db, op)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := a.store.Stage(context.Background(), op.ID, dir, digest, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Install(context.Background(), op, &staged); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := state.Skill{
		ID: detail.Skill.ID, Slug: detail.Skill.Slug, Name: detail.Skill.Name,
		Description: detail.Skill.Description, StoreDigest: digest, BaselineDigest: digest,
		CreatedAt: detail.Skill.CreatedAt, UpdatedAt: now,
	}
	b := state.Binding{SourceID: srcB.ID, RelativeDir: "skills/alpha", Digest: digest, SourceCommit: srcB.LastCommit, ImportedAt: now}
	if err := state.CommitReplace(a.db, s, b, op.ID); err != nil {
		t.Fatal(err)
	}
	op.SkillID = skillID
	// reproduce the crash state between the Baseline backup move and the
	// candidate move: the old Baseline sits in staging/<opID>/baseline-old
	// with its durable witness (the fixed skillstore witness format)
	base := filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID))
	backup := filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID), "baseline-old")
	if err := os.Rename(base, backup); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(backup)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	root, err := os.OpenRoot(backup)
	if err != nil {
		t.Fatal(err)
	}
	baseDigest, err := source.TreeDigestRoot(context.Background(), root)
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	// the canonical witness binds the operation, the slot, the witness
	// file's own identity (sampled after creation), the backup identity,
	// and the backup digest
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
		op.ID, wSt.Dev, wSt.Ino, st.Dev, st.Ino, baseDigest)
	if err := os.WriteFile(witnessPath, []byte(witness), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := reopenApp(t, a)
	res := importSkills(t, fresh, srcB.ID, "skills/alpha")
	if res.Items[0].Status != StatusAlreadyImported || res.Items[0].SkillID != skillID {
		t.Fatalf("item after interrupted-finalize convergence: %+v", res.Items[0])
	}
	if ops := openOperations(t, fresh); len(ops) != 0 {
		t.Fatalf("the replace terminal row must be cleared: %+v", ops)
	}
	if data, err := os.ReadFile(filepath.Join(a.StorePath, ".skillctl", "baselines", itoa(skillID), "SKILL.md")); err != nil ||
		string(data) != "---\nname: Alpha\ndescription: desc\n---\n# Alpha v2\n" {
		t.Fatalf("the advanced Baseline: %q, %v", data, err)
	}
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatal("the witnessed backup must be drained")
	}
	if _, err := os.Lstat(filepath.Join(a.StorePath, ".skillctl", "staging", itoa(op.ID))); !os.IsNotExist(err) {
		t.Fatal("the receipt-bound evidence must be cleaned")
	}
}
