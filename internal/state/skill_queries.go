package state

import (
	"database/sql"
	"time"
)

// BindingDetail is one Skill's Source Binding plus the Source display name,
// so an app-facing Skill view never needs a second query.
type BindingDetail struct {
	Binding
	SourceName string
}

// SkillDetail is one committed Skill with its optional Source Binding. A
// Skill without a Binding (possible after a later Detach) has a nil Binding.
type SkillDetail struct {
	Skill
	Binding *BindingDetail
}

// skillDetailSelect is the shared projection for Skill details: the Skill
// row LEFT JOINed with its Binding and the Binding's Source name. One join
// keeps every returned detail a coherent row.
const skillDetailSelect = `s.id, s.slug, s.name, s.description, s.store_digest,
	s.baseline_digest, s.created_at, s.updated_at, ` + skillSyncColumns + `,
	b.skill_id, b.source_id, b.relative_dir, b.digest, b.source_commit, b.imported_at,
	src.name, snap.skill_id`

const skillDetailFrom = `FROM skills s
	LEFT JOIN source_bindings b ON b.skill_id = s.id
	LEFT JOIN sources src ON src.id = b.source_id
	LEFT JOIN skill_snapshots snap ON snap.skill_id = s.id`

// scanner matches both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanSkillDetail scans one shared Skill-detail projection.
func scanSkillDetail(row scanner) (*SkillDetail, error) {
	var d SkillDetail
	var createdAt, updatedAt string
	var importedAt sql.NullString
	var bSkillID, bSourceID sql.NullInt64
	var bRelDir, bDigest, bCommit, srcName sql.NullString
	var snapSkillID sql.NullInt64
	syncDests, parseSync := skillSyncDests(&d.Skill)
	dests := append([]any{&d.Skill.ID, &d.Skill.Slug, &d.Skill.Name, &d.Skill.Description,
		&d.Skill.StoreDigest, &d.Skill.BaselineDigest, &createdAt, &updatedAt}, syncDests...)
	dests = append(dests, &bSkillID, &bSourceID, &bRelDir, &bDigest, &bCommit, &importedAt, &srcName, &snapSkillID)
	err := row.Scan(dests...)
	if err != nil {
		return nil, err
	}
	if err := parseSync(); err != nil {
		return nil, err
	}
	d.Skill.HasPreviousSnapshot = snapSkillID.Valid
	if d.Skill.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	if d.Skill.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, err
	}
	if !bSkillID.Valid {
		return &d, nil
	}
	d.Binding = &BindingDetail{
		Binding: Binding{
			SkillID:      bSkillID.Int64,
			SourceID:     bSourceID.Int64,
			RelativeDir:  bRelDir.String,
			Digest:       bDigest.String,
			SourceCommit: bCommit.String,
		},
		SourceName: srcName.String,
	}
	if d.Binding.ImportedAt, err = time.Parse(time.RFC3339Nano, importedAt.String); err != nil {
		return nil, err
	}
	return &d, nil
}

// GetSkillDetailByID returns one Skill with its Binding and Source name by
// numeric id, or sql.ErrNoRows when it does not exist.
func GetSkillDetailByID(db *sql.DB, id int64) (*SkillDetail, error) {
	return scanSkillDetail(db.QueryRow(`SELECT `+skillDetailSelect+` `+skillDetailFrom+` WHERE s.id = ?`, id))
}

// GetSkillDetailBySlug returns one Skill with its Binding and Source name by
// slug, or sql.ErrNoRows when no Skill carries the slug.
func GetSkillDetailBySlug(db *sql.DB, slug string) (*SkillDetail, error) {
	return scanSkillDetail(db.QueryRow(`SELECT `+skillDetailSelect+` `+skillDetailFrom+` WHERE s.slug = ?`, slug))
}

// GetSkillBySourceEntry returns the Skill bound to one Source Inventory
// entry (Source id plus relative directory), or sql.ErrNoRows when no
// Binding matches. The tuple is unique, so at most one Skill can match.
func GetSkillBySourceEntry(db *sql.DB, sourceID int64, relativeDir string) (*SkillDetail, error) {
	return scanSkillDetail(db.QueryRow(`SELECT `+skillDetailSelect+` `+skillDetailFrom+
		` WHERE b.source_id = ? AND b.relative_dir = ?`, sourceID, relativeDir))
}

// ListSkillDetails returns all committed Skills ordered by slug then id,
// each with its Binding and Source name.
func ListSkillDetails(db *sql.DB) ([]SkillDetail, error) {
	rows, err := db.Query(`SELECT ` + skillDetailSelect + ` ` + skillDetailFrom + ` ORDER BY s.slug, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillDetail
	for rows.Next() {
		d, err := scanSkillDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
