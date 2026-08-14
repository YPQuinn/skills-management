package app

import (
	"context"
	"time"

	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/sync"
)

// SyncSkills synchronizes every Skill bound to one Source as one batch:
// exactly one fresh coherent observation serves all its entries, and each
// Skill then runs its own safe-sync decision under one Store exclusive
// lock with its own filesystem and SQLite transaction, so one failure never
// rolls back another Skill's success. The Binding list is read before the
// observation, and every item's Skill facts are re-read under the held lock
// after recovery: a Binding that changed while the batch waited for the
// lock is never acted on through the stale list sample. A begun batch
// always returns every item outcome plus a summary; top-level errors are
// only request, lock, and recovery failures before item processing.
func (a *App) SyncSkills(ctx context.Context, sourceID int64) (*SyncSkillsResult, error) {
	if sourceID <= 0 {
		return nil, Errorf(CodeInvalidArgument, "a Source id is required")
	}
	details, err := state.ListSkillsBySource(a.db, sourceID)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing the Source %d bindings: %v", sourceID, err)
	}
	src, err := a.observeSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if !src.Available {
		return a.blockedUnavailableBatch(ctx, src, details), nil
	}
	items := make([]SyncItemResult, 0, len(details))
	blocked := false
	if err := a.withStoreLock(ctx, func() error {
		for _, d := range details {
			if blocked {
				items = append(items, SyncItemResult{
					SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionSync,
					Result: sync.ResultFailed, ErrorCode: CodeRecovery,
					ErrorMessage: "a previous item left an unresolved Store operation; later Store writes are blocked until recovery succeeds",
				})
				continue
			}
			// The Skill facts are re-read under the held lock and the
			// Binding is verified against the fresh observation: an item
			// whose Binding changed while the batch waited is out of this
			// batch's scope and is never synchronized through the stale
			// list sample.
			fresh, err := state.GetSkillDetailByID(a.db, d.Skill.ID)
			if err != nil {
				items = append(items, SyncItemResult{
					SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionSync,
					Result: sync.ResultFailed, ErrorCode: CodeInternal,
					ErrorMessage: "reading the Skill under the Store lock: " + err.Error(),
				})
				continue
			}
			if fresh.Binding == nil || fresh.Binding.SourceID != src.ID {
				item := SyncItemResult{SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionSync,
					Result:       sync.ResultSkipped,
					ErrorMessage: "the Skill is no longer bound to Source " + src.Name + "; re-check the Skill",
				}
				if fresh.Binding == nil {
					item.Status = sync.StatusUnbound
				}
				items = append(items, item)
				continue
			}
			out := a.actSync(ctx, src, *fresh)
			items = append(items, out.result)
			blocked = out.blocked
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &SyncSkillsResult{Items: items, Summary: SummarizeSyncItems(items)}, nil
}

// blockedUnavailableBatch reports every bound Skill of an unreachable
// Source: the last known relationship is retained and marked stale, and
// each item records a blocked outcome without touching the Store.
func (a *App) blockedUnavailableBatch(ctx context.Context, src *source.Source, details []state.SkillDetail) *SyncSkillsResult {
	items := make([]SyncItemResult, 0, len(details))
	for _, d := range details {
		started := time.Now().UTC()
		status := sync.Status(d.Skill.SyncStatus)
		if status == "" {
			status = sync.StatusUnchecked
		}
		item := SyncItemResult{
			SkillID: d.Skill.ID, Slug: d.Skill.Slug, Action: sync.ActionSync,
			Status: status, Stale: true, Result: sync.ResultBlocked,
			BeforeDigest: d.Skill.StoreDigest, Revision: src.LastCommit,
			ErrorMessage: "Source " + src.Name + " is not reachable: " + src.LastError,
		}
		if err := a.persistSyncStatus(d.Skill.ID, status, true); err != nil {
			item.ErrorCode = CodeInternal
		}
		if err := a.persistSyncOutcome(d.Skill.ID, item, started); err != nil {
			item.ErrorCode = CodeInternal
		}
		items = append(items, item)
	}
	return &SyncSkillsResult{Items: items, Summary: SummarizeSyncItems(items)}
}
