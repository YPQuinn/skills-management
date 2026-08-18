package state

import (
	"database/sql"
	"time"
)

// ManagedLink is one owned Target symlink: the Target, Skill, exact link
// path, the exact raw link target, and the time Skill Manager created or
// adopted it (decision 06). Ownership is provable only while this row
// exists and the filesystem entry still matches it.
type ManagedLink struct {
	ID            int64
	TargetID      int64
	SkillID       int64
	LinkPath      string
	RawTarget     string
	EstablishedAt time.Time
}

const managedLinkSelect = `id, target_id, skill_id, link_path, raw_target, established_at`

func scanManagedLink(row scanner) (*ManagedLink, error) {
	var l ManagedLink
	var established string
	if err := row.Scan(&l.ID, &l.TargetID, &l.SkillID, &l.LinkPath, &l.RawTarget, &established); err != nil {
		return nil, err
	}
	var err error
	if l.EstablishedAt, err = time.Parse(time.RFC3339Nano, established); err != nil {
		return nil, err
	}
	return &l, nil
}

// ManagedLinkByTargetSkill is one ledger row with the Skill's slug resolved.
type ManagedLinkByTargetSkill struct {
	SkillID   int64
	Slug      string
	LinkPath  string
	RawTarget string
}

// InsertManagedLink records one owned link and returns its id.
func InsertManagedLink(db *sql.DB, l ManagedLink) (int64, error) {
	res, err := db.Exec(`INSERT INTO managed_links
		(target_id, skill_id, link_path, raw_target, established_at)
		VALUES (?, ?, ?, ?, ?)`,
		l.TargetID, l.SkillID, l.LinkPath, l.RawTarget, timeToSQL(&l.EstablishedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ReplaceManagedLink records (or re-records, on explicit adoption) one
// owned link for a Target–Skill pair with its current raw target.
func ReplaceManagedLink(db *sql.DB, l ManagedLink) error {
	res, err := db.Exec(`INSERT INTO managed_links
		(target_id, skill_id, link_path, raw_target, established_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (target_id, skill_id) DO UPDATE SET
			link_path = excluded.link_path,
			raw_target = excluded.raw_target,
			established_at = excluded.established_at`,
		l.TargetID, l.SkillID, l.LinkPath, l.RawTarget, timeToSQL(&l.EstablishedAt))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// ListManagedLinksByTarget returns one Target's ledger rows ordered by the
// Skill slug, each with the Skill's current slug resolved.
func ListManagedLinksByTarget(db *sql.DB, targetID int64) ([]ManagedLinkByTargetSkill, error) {
	rows, err := db.Query(`SELECT ml.skill_id, s.slug, ml.link_path, ml.raw_target
		FROM managed_links ml JOIN skills s ON s.id = ml.skill_id
		WHERE ml.target_id = ? ORDER BY s.slug, s.id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedLinkByTargetSkill
	for rows.Next() {
		var l ManagedLinkByTargetSkill
		if err := rows.Scan(&l.SkillID, &l.Slug, &l.LinkPath, &l.RawTarget); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetManagedLink returns one Target–Skill ledger row, or sql.ErrNoRows.
func GetManagedLink(db *sql.DB, targetID, skillID int64) (*ManagedLink, error) {
	return scanManagedLink(db.QueryRow(`SELECT `+managedLinkSelect+`
		FROM managed_links WHERE target_id = ? AND skill_id = ?`, targetID, skillID))
}

// DeleteManagedLink removes one Target–Skill ledger row. The caller must
// have proven the filesystem entry no longer needs the claim (removed, or
// replaced by content Skill Manager no longer owns).
func DeleteManagedLink(db *sql.DB, targetID, skillID int64) error {
	_, err := db.Exec(`DELETE FROM managed_links WHERE target_id = ? AND skill_id = ?`, targetID, skillID)
	return err
}

// Link intent phases persisted with the unguessable slot name.
const (
	LinkPhasePlanned   = "planned"
	LinkPhasePrepared  = "prepared"
	LinkPhaseInstalled = "installed"
)

// LinkIntent is one durable link mutation intent recorded before the
// filesystem operation runs. SlotName is the operation-owned temporary
// path; Phase records how far the mutation progressed.
type LinkIntent struct {
	ID        int64
	TargetID  int64
	SkillID   int64
	Action    string
	LinkPath  string
	RawTarget string
	SlotName  string
	Phase     string
	CreatedAt time.Time
}

// InsertLinkIntent records one link intent and returns its id.
func InsertLinkIntent(db *sql.DB, i LinkIntent) (int64, error) {
	phase := i.Phase
	if phase == "" {
		phase = LinkPhasePlanned
	}
	res, err := db.Exec(`INSERT INTO link_intents
		(target_id, skill_id, action, link_path, raw_target, slot_name, phase, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		i.TargetID, i.SkillID, i.Action, i.LinkPath, i.RawTarget, i.SlotName, phase, timeToSQL(&i.CreatedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetLinkIntentPhase records how far one intent's filesystem mutation
// progressed, so recovery converges from the persisted slot and phase.
func SetLinkIntentPhase(db *sql.DB, id int64, phase string) error {
	return SetLinkIntentSlot(db, id, "", phase)
}

// SetLinkIntentSlot records the private isolation directory (when name is
// non-empty) and phase of one intent.
func SetLinkIntentSlot(db *sql.DB, id int64, slot, phase string) error {
	var res sql.Result
	var err error
	if slot == "" {
		res, err = db.Exec(`UPDATE link_intents SET phase = ? WHERE id = ?`, phase, id)
	} else {
		res, err = db.Exec(`UPDATE link_intents SET slot_name = ?, phase = ? WHERE id = ?`, slot, phase, id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// FinalizeCreateLedger atomically records one successfully created link
// and clears its intent, so a crash between the filesystem creation and
// the finalization leaves the intent for recovery.
func FinalizeCreateLedger(db *sql.DB, l ManagedLink, intentID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO managed_links
		(target_id, skill_id, link_path, raw_target, established_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (target_id, skill_id) DO UPDATE SET
			link_path = excluded.link_path,
			raw_target = excluded.raw_target,
			established_at = excluded.established_at`,
		l.TargetID, l.SkillID, l.LinkPath, l.RawTarget, timeToSQL(&l.EstablishedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM link_intents WHERE id = ?`, intentID); err != nil {
		return err
	}
	return tx.Commit()
}

// FinalizeRemoveLedger atomically relinquishes one Target–Skill ownership
// claim and clears its intent, after the filesystem entry was removed or
// proven foreign.
func FinalizeRemoveLedger(db *sql.DB, targetID, skillID, intentID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM managed_links WHERE target_id = ? AND skill_id = ?`, targetID, skillID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM link_intents WHERE id = ?`, intentID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLinkIntent removes one resolved intent row.
func DeleteLinkIntent(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM link_intents WHERE id = ?`, id)
	return err
}

// ListOpenLinkIntents returns every unfinished intent in id order, the
// deterministic recovery order.
func ListOpenLinkIntents(db *sql.DB) ([]LinkIntent, error) {
	return listLinkIntents(db, ``)
}

// ListOpenLinkIntentsByTarget returns one Target's unfinished intents in id
// order.
func ListOpenLinkIntentsByTarget(db *sql.DB, targetID int64) ([]LinkIntent, error) {
	return listLinkIntents(db, ` WHERE target_id = ?`, targetID)
}

func listLinkIntents(db *sql.DB, where string, args ...any) ([]LinkIntent, error) {
	rows, err := db.Query(`SELECT id, target_id, skill_id, action, link_path, raw_target, slot_name, phase, created_at
		FROM link_intents`+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LinkIntent
	for rows.Next() {
		var i LinkIntent
		var created string
		if err := rows.Scan(&i.ID, &i.TargetID, &i.SkillID, &i.Action, &i.LinkPath, &i.RawTarget, &i.SlotName, &i.Phase, &created); err != nil {
			return nil, err
		}
		if i.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
