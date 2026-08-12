package app

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"skillctl/internal/state"
)

// Skill is one app-facing Skill with its optional Source Binding. It never
// exposes SQLite rows or Store-internal paths; the Binding is shown by
// Source id and display name.
type Skill struct {
	ID             int64          `json:"id"`
	Slug           string         `json:"slug"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	StoreDigest    string         `json:"store_digest"`
	BaselineDigest string         `json:"baseline_digest"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Binding        *SourceBinding `json:"binding,omitempty"`
}

// SourceBinding is the app-facing view of one Skill's Source Binding.
type SourceBinding struct {
	SourceID     int64     `json:"source_id"`
	SourceName   string    `json:"source_name"`
	RelativeDir  string    `json:"relative_dir"`
	Digest       string    `json:"digest"`
	SourceCommit string    `json:"source_commit,omitempty"`
	ImportedAt   time.Time `json:"imported_at"`
}

// ListSkills returns all committed Skills in slug order with their Bindings.
func (a *App) ListSkills() ([]Skill, error) {
	details, err := state.ListSkillDetails(a.db)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Skills: %v", err)
	}
	out := make([]Skill, 0, len(details))
	for _, d := range details {
		out = append(out, appSkill(d))
	}
	return out, nil
}

// ShowSkill returns one Skill by numeric id with its Binding.
func (a *App) ShowSkill(id int64) (*Skill, error) {
	d, err := state.GetSkillDetailByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", id, err)
	}
	s := appSkill(*d)
	return &s, nil
}

// ResolveSkillArg maps a CLI argument to a Skill id: a numeric argument is
// the id itself, anything else is the unique slug. The numeric-first order
// matches ResolveSourceArg; a numeric-only slug is therefore not directly
// addressable by its digits.
func (a *App) ResolveSkillArg(arg string) (int64, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return id, nil
	}
	d, err := state.GetSkillDetailBySlug(a.db, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Errorf(CodeNotFound, "Skill %q not found", arg)
	}
	if err != nil {
		return 0, Errorf(CodeInternal, "resolving Skill %q: %v", arg, err)
	}
	return d.Skill.ID, nil
}

// appSkill converts one state detail into the app-facing Skill view.
func appSkill(d state.SkillDetail) Skill {
	s := Skill{
		ID:             d.Skill.ID,
		Slug:           d.Skill.Slug,
		Name:           d.Skill.Name,
		Description:    d.Skill.Description,
		StoreDigest:    d.Skill.StoreDigest,
		BaselineDigest: d.Skill.BaselineDigest,
		CreatedAt:      d.Skill.CreatedAt,
		UpdatedAt:      d.Skill.UpdatedAt,
	}
	if d.Binding != nil {
		s.Binding = &SourceBinding{
			SourceID:     d.Binding.SourceID,
			SourceName:   d.Binding.SourceName,
			RelativeDir:  d.Binding.RelativeDir,
			Digest:       d.Binding.Digest,
			SourceCommit: d.Binding.SourceCommit,
			ImportedAt:   d.Binding.ImportedAt,
		}
	}
	return s
}
