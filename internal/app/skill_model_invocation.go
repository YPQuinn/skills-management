package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// FrontmatterDisableModelInvocation is the SKILL.md frontmatter field that,
// when true, bars an Agent from invoking the Skill on its own initiative:
// the Skill must be triggered by the user. Its absence means false.
const FrontmatterDisableModelInvocation = "disable-model-invocation"

// SetSkillModelInvocationDisabled sets (disabled) or clears (enabled) the
// disable-model-invocation frontmatter field of one Skill's live SKILL.md and
// persists the resulting Store digest and Sync Status. The edit is a
// deliberate in-place local Store modification: a bound Skill whose Store
// content now diverges from its accepted Source evaluates to store_changed,
// exactly like any other local SKILL.md edit. The refreshed Skill content is
// returned so the caller reflects the persisted state without a second read.
func (a *App) SetSkillModelInvocationDisabled(ctx context.Context, skillID int64, disabled bool) (*SkillContent, error) {
	if _, err := state.GetSkillDetailByID(a.db, skillID); errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	} else if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	if err := a.withStoreLock(ctx, func() error {
		d, err := state.GetSkillDetailByID(a.db, skillID)
		if errors.Is(err, sql.ErrNoRows) {
			return Errorf(CodeNotFound, "Skill %d not found", skillID)
		}
		if err != nil {
			return Errorf(CodeInternal, "reading Skill %d under the Store lock: %v", skillID, err)
		}
		dir, err := a.store.SkillDir(d.Skill.Slug)
		if err != nil {
			return Errorf(CodeInternal, "resolving the Store path: %v", err)
		}
		docPath := filepath.Join(dir, "SKILL.md")
		info, err := os.Stat(docPath)
		if errors.Is(err, os.ErrNotExist) {
			return Errorf(CodeNotFound, "SKILL.md is not present in the Skill Store for Skill %q", d.Skill.Slug)
		}
		if err != nil {
			return Errorf(CodeInternal, "reading SKILL.md: %v", err)
		}
		data, err := os.ReadFile(docPath)
		if err != nil {
			return Errorf(CodeInternal, "reading SKILL.md: %v", err)
		}
		edited, changed, err := source.SetScalarField(data, FrontmatterDisableModelInvocation, "true", !disabled)
		if err != nil {
			return Errorf(CodeConflict, "Skill %q has invalid frontmatter: %v", d.Skill.Slug, err)
		}
		if !changed {
			return nil
		}
		if err := writeFileInDir(dir, "SKILL.md", edited, info.Mode().Perm()); err != nil {
			return Errorf(CodeInternal, "writing SKILL.md: %v", err)
		}
		digest, err := source.TreeDigest(ctx, dir)
		if err != nil {
			return Errorf(CodeInternal, "recomputing the Store digest of Skill %q: %v", d.Skill.Slug, err)
		}
		now := time.Now().UTC()
		if err := state.SetSkillStoreDigest(a.db, skillID, d.Skill.Slug, d.Skill.StoreDigest, digest, now); err != nil {
			return Errorf(CodeInternal, "recording the Store digest of Skill %d: %v", skillID, err)
		}
		status := sync.StatusUnbound
		if d.Binding != nil {
			status = sync.RecheckStatus(sync.StatusInput{
				Bound:        true,
				SourceDigest: d.Binding.Digest,
				StoreDigest:  digest,
				Baseline:     d.Skill.BaselineDigest,
			})
		}
		return a.persistSyncStatus(skillID, status, false)
	}); err != nil {
		return nil, err
	}
	return a.SkillContent(skillID)
}

// writeFileInDir atomically replaces name inside dir: it writes a temp file in
// the same directory, fsyncs it, renames it over the target, and fsyncs the
// directory, so a crash leaves either the whole old or the whole new content,
// never a truncated file. It never follows a symlink at name.
func writeFileInDir(dir, name string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(dir, "."+name+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}
