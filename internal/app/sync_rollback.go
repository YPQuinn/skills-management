package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// Rollback restores the single previous snapshot into the live path through
// the durable keep-baseline replace journal: the live content is re-proven
// against the persisted Store digest before anything moves, the snapshot is
// digest-validated by staging, and the pre-rollback live tree becomes the
// new single previous snapshot. Binding, Group, Assignment, and Source
// state are untouched; the Sync Status is recomputed afterwards.
func (a *App) Rollback(ctx context.Context, skillID int64) (*SyncItemResult, error) {
	d, err := state.GetSkillDetailByID(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeNotFound, "Skill %d not found", skillID)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading Skill %d: %v", skillID, err)
	}
	snapshot, err := state.GetSnapshot(a.db, skillID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Errorf(CodeConflict, "Skill %q has no previous snapshot to roll back to", d.Skill.Slug)
	}
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the previous snapshot of Skill %d: %v", skillID, err)
	}
	started := time.Now().UTC()
	item := SyncItemResult{SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionRollback,
		BeforeDigest: d.Skill.StoreDigest, Revision: snapshot.SourceCommit}
	if err := a.withStoreLock(ctx, func() error {
		// Recovery ran first; the Skill facts and the snapshot metadata are
		// re-read under the held lock, so a rollback never replaces a live
		// tree against a stale persisted digest or a superseded snapshot.
		fresh, err := state.GetSkillDetailByID(a.db, skillID)
		if err != nil {
			return Errorf(CodeInternal, "reading Skill %d under the Store lock: %v", skillID, err)
		}
		snapshot, err = state.GetSnapshot(a.db, skillID)
		if errors.Is(err, sql.ErrNoRows) {
			item.Result, item.ErrorMessage = sync.ResultBlocked, "the previous snapshot is gone; there is nothing to roll back to"
			return nil
		}
		if err != nil {
			return Errorf(CodeInternal, "reading the previous snapshot of Skill %d under the Store lock: %v", skillID, err)
		}
		item.Revision = snapshot.SourceCommit
		d = fresh
		storeDigest, missing, invalid := a.storeTreeState(ctx, d.Skill.Slug)
		switch {
		case missing:
			item.Result, item.ErrorMessage = sync.ResultBlocked, "the Store content is missing; there is nothing to capture as the new previous snapshot"
			return nil
		case invalid:
			item.Result, item.ErrorMessage = sync.ResultBlocked, "the Store content cannot be read"
			return nil
		case storeDigest != d.Skill.StoreDigest:
			item.Result, item.ErrorMessage = sync.ResultBlocked,
				"the Store content changed since it was last checked; check the Skill first"
			return nil
		case storeDigest == snapshot.Digest:
			item.Result, item.ErrorMessage = sync.ResultBlocked, "the live content already equals the previous snapshot"
			return nil
		}
		prevDir := a.store.PreviousDir(skillID)
		if fail := a.replaceSkillContent(ctx, d, nil, nil, prevDir, snapshot.Digest, modeReplaceKeep, "rollback", ""); fail.code != "" {
			item.Result = sync.ResultFailed
			item.ErrorCode, item.ErrorMessage = fail.code, fail.message
			return nil
		}
		item.Result = sync.ResultRolledBack
		item.AfterDigest = snapshot.Digest
		// The relationship is evaluated from the latest persisted Source
		// inventory, issues, and availability — never from the Binding
		// digest, which records the accepted content and would silently
		// mask a Source that moved, lost the entry, or became invalid
		// since the last acceptance.
		if d.Binding != nil {
			srcRow, err := state.GetSource(a.db, d.Binding.SourceID)
			if err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
				return nil
			}
			status, stale, _, _, err := a.evaluateSkillSync(ctx, *d, srcRow)
			if err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
				return nil
			}
			item.Status, item.Stale = status, stale
			if err := a.persistSyncStatus(d.Skill.ID, status, stale); err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
			}
		} else {
			item.Status = sync.StatusUnbound
			if err := a.persistSyncStatus(d.Skill.ID, sync.StatusUnbound, false); err != nil {
				item.ErrorCode, item.ErrorMessage = CodeInternal, err.Error()
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
		item.ErrorCode = CodeInternal
	}
	return &item, nil
}
