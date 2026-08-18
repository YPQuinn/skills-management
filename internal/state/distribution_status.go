package state

import (
	"database/sql"
	"time"
)

// DistributionItem is the persisted Distribution Status of one Skill at
// one Target: desired and last-observed presence, the observation details,
// freshness, and the latest reconciliation outcome.
type DistributionItem struct {
	TargetID       int64
	SkillID        int64
	Slug           string
	Desired        string
	Observed       string
	Managed        bool
	Adoptable      bool
	NodeKind       string
	RawTarget      string
	ResolvedTarget string
	Stale          bool
	InspectedAt    *time.Time
	LastResult     string
	LastError      string
}

// TargetDistribution is one Target's persisted inspection and
// reconciliation outcome metadata.
type TargetDistribution struct {
	InspectedAt    *time.Time
	InspectedStale bool
	InspectError   string
	LastResult     string
	LastStartedAt  *time.Time
	LastCompleted  *time.Time
	LastError      string
}

// ReplaceDistributionItems persists one coherent inspection wholesale: the
// given items become the Target's complete observation set, and any stored
// row for a relation the inspection no longer covers is removed.
func ReplaceDistributionItems(db *sql.DB, targetID int64, items []DistributionItem) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM distribution_items WHERE target_id = ?`, targetID); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(`INSERT INTO distribution_items
			(target_id, skill_id, desired, observed, managed, adoptable,
			 node_kind, raw_target, resolved_target, stale, inspected_at,
			 last_result, last_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			targetID, it.SkillID, it.Desired, it.Observed, boolInt(it.Managed), boolInt(it.Adoptable),
			it.NodeKind, it.RawTarget, it.ResolvedTarget, boolInt(it.Stale),
			timeToSQL(it.InspectedAt), it.LastResult, it.LastError); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkDistributionItemsStale flags every stored observation of one Target
// stale with the given failure detail, used when an inspection cannot
// complete: the previous observation is retained, never partially
// overwritten.
func MarkDistributionItemsStale(db *sql.DB, targetID int64, errMsg string) error {
	_, err := db.Exec(`UPDATE distribution_items SET stale = 1, last_error = ?
		WHERE target_id = ?`, errMsg, targetID)
	return err
}

// ListDistributionItems returns one Target's stored observation ordered by
// Skill slug.
func ListDistributionItems(db *sql.DB, targetID int64) ([]DistributionItem, error) {
	rows, err := db.Query(`SELECT di.skill_id, s.slug, di.desired, di.observed, di.managed,
		di.adoptable, di.node_kind, di.raw_target, di.resolved_target,
		di.stale, di.inspected_at, di.last_result, di.last_error
		FROM distribution_items di JOIN skills s ON s.id = di.skill_id
		WHERE di.target_id = ? ORDER BY s.slug, s.id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DistributionItem
	for rows.Next() {
		var it DistributionItem
		var inspected any
		if err := rows.Scan(&it.SkillID, &it.Slug, &it.Desired, &it.Observed,
			&it.Managed, &it.Adoptable, &it.NodeKind, &it.RawTarget, &it.ResolvedTarget,
			&it.Stale, &inspected, &it.LastResult, &it.LastError); err != nil {
			return nil, err
		}
		if it.InspectedAt, err = timeFromSQL(inspected); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateDistributionItemOutcome persists one item's latest reconciliation
// outcome on its stored row. A row is created if the relation had no
// previous observation.
func UpdateDistributionItemOutcome(db *sql.DB, targetID, skillID int64, desired, observed, result, errMsg string, inspectedAt time.Time) error {
	if _, err := db.Exec(`INSERT INTO distribution_items
		(target_id, skill_id, desired, observed, stale, inspected_at, last_result, last_error)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT (target_id, skill_id) DO UPDATE SET
			desired = excluded.desired, observed = excluded.observed,
			stale = 0, inspected_at = excluded.inspected_at,
			last_result = excluded.last_result, last_error = excluded.last_error`,
		targetID, skillID, desired, observed, timeToSQL(&inspectedAt), result, errMsg); err != nil {
		return err
	}
	return nil
}

// DeleteDistributionItem removes one relation's stored observation row
// once nothing remains to observe: the desired state is absent, the entry
// is missing, and the ownership record has been cleared (decision 06).
func DeleteDistributionItem(db *sql.DB, targetID, skillID int64) error {
	_, err := db.Exec(`DELETE FROM distribution_items WHERE target_id = ? AND skill_id = ?`, targetID, skillID)
	return err
}

// UpdateTargetInspection persists one Target's completed inspection.
func UpdateTargetInspection(db *sql.DB, targetID int64, d TargetDistribution) error {
	res, err := db.Exec(`UPDATE targets SET
		last_inspected_at = ?, last_inspected_stale = ?, last_inspected_error = ?,
		updated_at = ?
		WHERE id = ?`,
		timeToSQL(d.InspectedAt), boolInt(d.InspectedStale), d.InspectError,
		timeToSQL(nowUTC()), targetID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateTargetOutcome persists one Target's latest reconciliation outcome.
func UpdateTargetOutcome(db *sql.DB, targetID int64, result string, started, completed time.Time, errMsg string) error {
	res, err := db.Exec(`UPDATE targets SET
		last_distribute_result = ?, last_distribute_started_at = ?,
		last_distribute_completed_at = ?, last_distribute_error = ?
		WHERE id = ?`,
		result, timeToSQL(&started), timeToSQL(&completed), errMsg, targetID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
