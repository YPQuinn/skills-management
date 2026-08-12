package state

import (
	"strings"
	"testing"

	"skillctl/internal/skillstore"
)

func TestCommitReplaceRejectsSlugChange(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	renamed := testSkill("beta")
	renamed.ID = id
	if err := CommitReplace(db, renamed, testBinding(srcID, id), op.ID); err == nil {
		t.Fatal("replace with a different slug: want error")
	}
	// the operation stays pending and the Skill keeps its slug
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("refused replace must leave the intent pending: %+v", ops)
	}
	if _, _, err := GetSkillBySlug(db, "alpha"); err != nil {
		t.Fatalf("original slug must survive: %v", err)
	}
}

func TestCommitReplaceRejectsMissingSkill(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindReplace, OldDigest: "store-digest", NewDigest: "new-digest",
	})
	missing := testSkill("alpha")
	missing.ID = 999
	if err := CommitReplace(db, missing, testBinding(srcID, 999), op.ID); err == nil {
		t.Fatal("replacing a missing Skill: want error")
	}
}

func TestCommitReplaceRejectsMissingBinding(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindReplace, OldDigest: "store-digest", NewDigest: "new-digest",
	})
	// the Skill exists but has no Binding yet
	if _, err := db.Exec(`INSERT INTO skills
		(slug, name, description, store_digest, baseline_digest, created_at, updated_at)
		VALUES ('alpha', 'A', '', 'store-digest', 'store-digest', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	updated := testSkill("alpha")
	updated.ID = 1
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b := testBinding(srcID, 1)
	b.Digest = "new-digest"
	if err := CommitReplace(db, updated, b, op.ID); err == nil {
		t.Fatal("replacing a Skill without a Binding: want error")
	}
}

func TestCommitReplaceRejectsWrongKind(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindImport,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b2 := testBinding(srcID, id)
	b2.Digest = "new-digest"
	if err := CommitReplace(db, updated, b2, op.ID); err == nil {
		t.Fatal("replace against an import intent: want error")
	}
}

// TestCommitReplaceRejectsMismatchedBaseline proves a replace cannot commit
// when the Skill's BaselineDigest diverges from its StoreDigest.
func TestCommitReplaceRejectsMismatchedBaseline(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "different-baseline"
	b := testBinding(srcID, id)
	b.Digest = "new-digest"
	if err := CommitReplace(db, updated, b, op.ID); err == nil {
		t.Fatal("replace with mismatched baseline digest: want error")
	}
	// the Skill and operation must be unchanged
	skill, _, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "store-digest" {
		t.Fatalf("a refused replace must not change the Skill: %+v", skill)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("a refused replace must leave the operation pending: %+v", ops)
	}
}

// TestCommitReplaceRejectsMismatchedBindingDigest proves a replace cannot
// commit when the Binding's Digest diverges from the Skill's StoreDigest.
func TestCommitReplaceRejectsMismatchedBindingDigest(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "store-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b := testBinding(srcID, id)
	b.Digest = "different-binding-digest"
	if err := CommitReplace(db, updated, b, op.ID); err == nil {
		t.Fatal("replace with mismatched binding digest: want error")
	}
	skill, _, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "store-digest" {
		t.Fatalf("a refused replace must not change the Skill: %+v", skill)
	}
}

// TestCommitReplaceRejectsStaleOldDigest proves a replace cannot commit when
// the operation's OldDigest does not match the currently persisted
// StoreDigest, so SQLite cannot bless a different transition than the
// filesystem journal.
func TestCommitReplaceRejectsStaleOldDigest(t *testing.T) {
	db := openTestDB(t)
	srcID := testSourceID(t, db)
	id, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(srcID, 0))
	if err != nil {
		t.Fatal(err)
	}
	op := insertOperation(t, db, skillstore.Operation{
		SkillID: id, Slug: "alpha", Kind: skillstore.KindReplace,
		OldDigest: "stale-old-digest", NewDigest: "new-digest",
	})
	updated := testSkill("alpha")
	updated.ID = id
	updated.StoreDigest = "new-digest"
	updated.BaselineDigest = "new-digest"
	b := testBinding(srcID, id)
	b.Digest = "new-digest"
	if err := CommitReplace(db, updated, b, op.ID); err == nil {
		t.Fatal("replace with stale old digest: want error")
	}
	// nothing was committed
	skill, binding, err := GetSkillBySlug(db, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if skill.StoreDigest != "store-digest" || binding.SourceCommit != "abc123" {
		t.Fatalf("a refused replace must roll back Skill and Binding: %+v, %+v", skill, binding)
	}
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Phase != skillstore.PhasePending {
		t.Fatalf("a stale operation must remain pending: %+v", ops)
	}
}

func TestOperationCRUD(t *testing.T) {
	db := openTestDB(t)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].ID != op.ID || ops[0].SkillID != 0 || ops[0].Kind != skillstore.KindImport {
		t.Fatalf("listed operation: %+v", ops)
	}
	if err := DeleteOperationIntent(db, op); err != nil {
		t.Fatal(err)
	}
	ops, err = ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("operations after delete: %+v", ops)
	}
}

// TestListOpenOperationsRoundsTripSlug pins the journal's slug column so a
// later recovery validates it against the slug grammar before touching any
// Store path.
func TestListOpenOperationsRoundsTripSlug(t *testing.T) {
	db := openTestDB(t)
	op := insertOperation(t, db, skillstore.Operation{
		Slug: "alpha", Kind: skillstore.KindImport, NewDigest: "d",
	})
	ops, err := ListOpenOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Slug != "alpha" || !strings.EqualFold(ops[0].Slug, op.Slug) {
		t.Fatalf("operation slug round trip: %+v", ops)
	}
}
