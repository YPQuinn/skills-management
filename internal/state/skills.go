package state

import (
	"database/sql"
	"fmt"
	"time"

	"skillctl/internal/skillstore"
)

// Skill is one committed Skill row: its unique slug, frontmatter-derived
// name and description, the canonical digest of its Store tree, and the
// accepted-content digest anchoring its Synchronization Baseline.
type Skill struct {
	ID             int64
	Slug           string
	Name           string
	Description    string
	StoreDigest    string
	BaselineDigest string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Binding is one Skill's Source Binding: the upstream Source entry the
// Skill was accepted from, the accepted content digest, and the observed
// Source commit at import time.
type Binding struct {
	SkillID      int64
	SourceID     int64
	RelativeDir  string
	Digest       string
	SourceCommit string
	ImportedAt   time.Time
}

// InsertSkillAndBinding persists one committed Skill and its Source Binding
// in one transaction and returns the Skill id.
func InsertSkillAndBinding(db *sql.DB, s Skill, b Binding) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id, err := insertSkillTx(tx, s)
	if err != nil {
		return 0, err
	}
	if err := insertBindingTx(tx, id, b); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateSkillAndBinding updates one committed Skill and its Binding in one
// transaction: the replace path, where content and Binding change while
// Skill identity, slug, and relationships are retained. The update is a
// compare-and-set: exactly one existing Skill row with the same slug must
// match, so a stale update or a slug change fails instead of silently
// applying.
func UpdateSkillAndBinding(db *sql.DB, s Skill, b Binding) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE skills SET
		name = ?, description = ?, store_digest = ?, baseline_digest = ?, updated_at = ?
		WHERE id = ? AND slug = ?`,
		s.Name, s.Description, s.StoreDigest, s.BaselineDigest,
		timeToSQL(&s.UpdatedAt), s.ID, s.Slug)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("updating Skill %d (%s): no matching row or slug changed", s.ID, s.Slug)
	}
	res, err = tx.Exec(`UPDATE source_bindings SET
		source_id = ?, relative_dir = ?, digest = ?, source_commit = ?, imported_at = ?
		WHERE skill_id = ?`,
		b.SourceID, b.RelativeDir, b.Digest, b.SourceCommit,
		timeToSQL(&b.ImportedAt), s.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("updating Skill %d: Source Binding is missing", s.ID)
	}
	return tx.Commit()
}

// CommitImport persists a committed import atomically: the Skill row, its
// Binding, and the operation's phase transition to 'committed' (also fixing
// the operation's Skill id) in one transaction. It returns the new Skill
// id. The operation must be the pending import intent being committed;
// replaying an already-committed or foreign operation fails instead of
// committing a Skill without a journal transition.
func CommitImport(db *sql.DB, s Skill, b Binding, opID int64) (int64, error) {
	// The accepted identity is impossible unless the new content digest
	// is identical across the Skill's Store and Baseline and the Binding;
	// the pending operation's NewDigest is matched by
	// markOperationCommittedTx. Without this, SQLite could bless content
	// the filesystem never proved.
	if s.StoreDigest == "" || s.StoreDigest != s.BaselineDigest || s.StoreDigest != b.Digest {
		return 0, fmt.Errorf("CommitImport %q: impossible accepted identity: store %q, baseline %q, binding %q",
			s.Slug, s.StoreDigest, s.BaselineDigest, b.Digest)
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id, err := insertSkillTx(tx, s)
	if err != nil {
		return 0, err
	}
	if err := insertBindingTx(tx, id, b); err != nil {
		return 0, err
	}
	expected := skillstore.Operation{
		ID: opID, Slug: s.Slug, Kind: skillstore.KindImport, NewDigest: s.StoreDigest,
	}
	if err := markOperationCommittedTx(tx, expected, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// CommitReplace persists a committed replace atomically: the Skill and
// Binding updates, the previous-snapshot metadata, and the operation's
// phase transition in one transaction. The Skill update is a
// compare-and-set that retains the immutable slug, the Binding update must
// affect exactly the one existing Binding, and the operation must be the
// pending replace intent being committed.
func CommitReplace(db *sql.DB, s Skill, b Binding, opID int64) error {
	// The accepted identity is impossible unless the new content digest
	// is identical across the Skill's Store and Baseline and the Binding;
	// the pending operation's NewDigest is matched by
	// markOperationCommittedTx.
	if s.StoreDigest == "" || s.StoreDigest != s.BaselineDigest || s.StoreDigest != b.Digest {
		return fmt.Errorf("CommitReplace %q: impossible accepted identity: store %q, baseline %q, binding %q",
			s.Slug, s.StoreDigest, s.BaselineDigest, b.Digest)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldStoreDigest, oldSourceCommit string
	if err := tx.QueryRow(`SELECT s.store_digest, b.source_commit
		FROM skills s JOIN source_bindings b ON b.skill_id = s.id
		WHERE s.id = ? AND s.slug = ?`, s.ID, s.Slug).Scan(&oldStoreDigest, &oldSourceCommit); err != nil {
		return fmt.Errorf("reading Skill %d and its Source Binding: %v", s.ID, err)
	}
	// The replace operation must describe the transition the Skill Store
	// is actually in: its OldDigest must match the currently persisted
	// StoreDigest before any mutation, so SQLite cannot bless a different
	// transition than the filesystem journal.
	var opOldDigest string
	if err := tx.QueryRow(`SELECT old_digest FROM store_operations WHERE id = ? AND phase = ?`,
		opID, skillstore.PhasePending).Scan(&opOldDigest); err != nil {
		return fmt.Errorf("reading pending replace operation %d: %v", opID, err)
	}
	if opOldDigest != oldStoreDigest {
		return fmt.Errorf("replacing Skill %d (%s): operation old digest %q does not match the persisted Store digest %q",
			s.ID, s.Slug, opOldDigest, oldStoreDigest)
	}
	res, err := tx.Exec(`UPDATE skills SET
		name = ?, description = ?, store_digest = ?, baseline_digest = ?, updated_at = ?
		WHERE id = ? AND slug = ? AND store_digest = ?`,
		s.Name, s.Description, s.StoreDigest, s.BaselineDigest,
		timeToSQL(&s.UpdatedAt), s.ID, s.Slug, oldStoreDigest)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("replacing Skill %d (%s): no matching row or slug changed", s.ID, s.Slug)
	}
	res, err = tx.Exec(`UPDATE source_bindings SET
		source_id = ?, relative_dir = ?, digest = ?, source_commit = ?, imported_at = ?
		WHERE skill_id = ?`,
		b.SourceID, b.RelativeDir, b.Digest, b.SourceCommit,
		timeToSQL(&b.ImportedAt), s.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("replacing Skill %d: Source Binding is missing", s.ID)
	}
	if err := upsertSnapshotTx(tx, s.ID, opID, oldSourceCommit); err != nil {
		return err
	}
	expected := skillstore.Operation{
		ID: opID, SkillID: s.ID, Slug: s.Slug, Kind: skillstore.KindReplace,
		OldDigest: oldStoreDigest, NewDigest: s.StoreDigest,
	}
	if err := markOperationCommittedTx(tx, expected, s.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSkillBySlug returns one committed Skill with its Binding, or
// sql.ErrNoRows when no Skill carries the slug. A Skill without a Binding
// (possible after a later Detach) returns a nil Binding.
func GetSkillBySlug(db *sql.DB, slug string) (*Skill, *Binding, error) {
	var s Skill
	var createdAt, updatedAt string
	err := db.QueryRow(`SELECT id, slug, name, description, store_digest, baseline_digest, created_at, updated_at
		FROM skills WHERE slug = ?`, slug).
		Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.StoreDigest, &s.BaselineDigest,
			&createdAt, &updatedAt)
	if err != nil {
		return nil, nil, err
	}
	s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, nil, err
	}
	s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, nil, err
	}
	b, err := getBinding(db, s.ID)
	if err != nil {
		return nil, nil, err
	}
	return &s, b, nil
}

// ListSkills returns all committed Skills ordered by slug.
func ListSkills(db *sql.DB) ([]Skill, error) {
	rows, err := db.Query(`SELECT id, slug, name, description, store_digest, baseline_digest, created_at, updated_at
		FROM skills ORDER BY slug, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.StoreDigest, &s.BaselineDigest,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		if s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func insertSkillTx(tx *sql.Tx, s Skill) (int64, error) {
	res, err := tx.Exec(`INSERT INTO skills
		(slug, name, description, store_digest, baseline_digest, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.Slug, s.Name, s.Description, s.StoreDigest, s.BaselineDigest,
		timeToSQL(&s.CreatedAt), timeToSQL(&s.UpdatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func insertBindingTx(tx *sql.Tx, skillID int64, b Binding) error {
	_, err := tx.Exec(`INSERT INTO source_bindings
		(skill_id, source_id, relative_dir, digest, source_commit, imported_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		skillID, b.SourceID, b.RelativeDir, b.Digest, b.SourceCommit,
		timeToSQL(&b.ImportedAt))
	return err
}

func getBinding(db *sql.DB, skillID int64) (*Binding, error) {
	var b Binding
	var importedAt string
	err := db.QueryRow(`SELECT skill_id, source_id, relative_dir, digest, source_commit, imported_at
		FROM source_bindings WHERE skill_id = ?`, skillID).
		Scan(&b.SkillID, &b.SourceID, &b.RelativeDir, &b.Digest, &b.SourceCommit, &importedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.ImportedAt, err = time.Parse(time.RFC3339Nano, importedAt)
	return &b, err
}
