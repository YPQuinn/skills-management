package state

import (
	"database/sql"
	"fmt"
	"time"
)

// SnapshotReasonReplace is the reason recorded for a previous snapshot
// rotated by an explicit Replace. Synchronization actions record their own
// reasons (accept_source, sync, rollback).
const SnapshotReasonReplace = "replace"

// Snapshot is the durable metadata of one Skill's single previous snapshot
// (decision 05): the content digest, the Source evidence at the time the
// snapshot was rotated, the reason, and the time. The physical tree lives
// at .skillctl/previous/<skillID>; a later rollback validates the tree
// against this metadata before using it.
type Snapshot struct {
	SkillID      int64
	Digest       string
	SourceCommit string
	Reason       string
	CreatedAt    time.Time
}

// GetSnapshot returns the previous-snapshot metadata of one Skill, or
// sql.ErrNoRows when the Skill has no previous snapshot.
func GetSnapshot(db *sql.DB, skillID int64) (*Snapshot, error) {
	var s Snapshot
	var createdAt string
	err := db.QueryRow(`SELECT skill_id, digest, source_commit, reason, created_at
		FROM skill_snapshots WHERE skill_id = ?`, skillID).
		Scan(&s.SkillID, &s.Digest, &s.SourceCommit, &s.Reason, &createdAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// upsertSnapshotTx persists the previous-snapshot metadata of a committed
// Replace atomically inside the caller's transaction: the digest is the
// replaced operation's old digest and the Source evidence is the replaced
// Binding's source commit (read before the Binding was updated). At most
// one row exists per Skill; a later Replace supersedes it.
func upsertSnapshotTx(tx *sql.Tx, skillID, opID int64, sourceCommit, reason string) error {
	var oldDigest string
	if err := tx.QueryRow(`SELECT old_digest FROM store_operations WHERE id = ?`, opID).Scan(&oldDigest); err != nil {
		return fmt.Errorf("reading replace operation %d: %v", opID, err)
	}
	now := time.Now().UTC()
	_, err := tx.Exec(`INSERT INTO skill_snapshots (skill_id, digest, source_commit, reason, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(skill_id) DO UPDATE SET
			digest = excluded.digest,
			source_commit = excluded.source_commit,
			reason = excluded.reason,
			created_at = excluded.created_at`,
		skillID, oldDigest, sourceCommit, reason, timeToSQL(&now))
	return err
}
