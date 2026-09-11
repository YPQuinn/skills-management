package app

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"skillctl/internal/source"
	"skillctl/internal/state"
)

// SkillContent is the live Skill Store copy of one Skill's SKILL.md.
type SkillContent struct {
	SkillID     int64
	Slug        string
	Path        string // "SKILL.md"
	Frontmatter []source.SkillField
	Body        string
}

// SkillContent returns the live Skill Store SKILL.md of one Skill by
// numeric id.
func (a *App) SkillContent(id int64) (*SkillContent, error) {
	d, err := state.GetSkillDetailByID(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", id)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", id, err)
	}
	dir, err := a.store.SkillDir(d.Skill.Slug)
	if err != nil {
		return nil, Errorf(CodeInternal, "resolving the Store path: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Errorf(CodeNotFound, "SKILL.md is not present in the Skill Store for Skill %q", d.Skill.Slug)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading SKILL.md: %v", err)
	}
	doc, err := source.ParseSkillDocument(data)
	if err != nil {
		return nil, Errorf(CodeConflict, "Skill %q has invalid frontmatter: %v", d.Skill.Slug, err)
	}
	return &SkillContent{
		SkillID:     d.Skill.ID,
		Slug:        d.Skill.Slug,
		Path:        "SKILL.md",
		Frontmatter: doc.Fields,
		Body:        doc.Body,
	}, nil
}
