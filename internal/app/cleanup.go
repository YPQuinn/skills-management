package app

import (
	"context"
	"sort"

	"skillctl/internal/lock"
	"skillctl/internal/state"
)

// CleanupLink is one Managed Link considered by a destructive cleanup:
// the Target, the Skill slug, and the reconciliation result after an
// attempted verified removal.
type CleanupLink struct {
	TargetID   int64  `json:"target_id"`
	TargetName string `json:"target_name"`
	SkillID    int64  `json:"skill_id"`
	Slug       string `json:"slug"`
	Result     string `json:"result,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

// SourceDeletePreview lists Skills that must be detached before a Source
// can be deleted.
type SourceDeletePreview struct {
	SourceID       int64      `json:"source_id"`
	Name           string     `json:"name"`
	BoundSkills    []SkillRef `json:"bound_skills"`
	RequiresDetach bool       `json:"requires_detach"`
}

// SkillDeletePreview is the replacement-style impact plus Managed Links
// that explicit Skill deletion would clean up.
type SkillDeletePreview struct {
	Skill      SkillRef       `json:"skill"`
	Groups     []GroupRef     `json:"groups"`
	Targets    []TargetImpact `json:"targets"`
	Links      []CleanupLink  `json:"links"`
	Referenced bool           `json:"referenced"`
}

// TargetDeletePreview lists Assignments and Managed Links that Target
// deletion would clean up. The container is never deleted.
type TargetDeletePreview struct {
	Target      TargetRef        `json:"target"`
	Assignments []AssignmentView `json:"assignments"`
	Links       []CleanupLink    `json:"links"`
}

// GroupDeletePreview lists Targets that assign the Group.
type GroupDeletePreview struct {
	Group    GroupRef    `json:"group"`
	Targets  []TargetRef `json:"targets"`
	Assigned bool        `json:"assigned"`
}

// SourceDeleteResult is the outcome of a successful Source deletion.
type SourceDeleteResult struct {
	SourceID int64      `json:"source_id"`
	Name     string     `json:"name"`
	Detached []SkillRef `json:"detached"`
}

// SkillDeleteResult is the outcome of a successful Skill deletion.
type SkillDeleteResult struct {
	Skill              SkillRef      `json:"skill"`
	RemovedAssignments int           `json:"removed_assignments"`
	RemovedMemberships int           `json:"removed_memberships"`
	Links              []CleanupLink `json:"links"`
}

// TargetDeleteResult is the outcome of a successful Target deletion.
type TargetDeleteResult struct {
	Target TargetRef     `json:"target"`
	Links  []CleanupLink `json:"links"`
}

// GroupDeleteResult is the outcome of a successful Group deletion.
type GroupDeleteResult struct {
	Group      GroupRef    `json:"group"`
	Unassigned []TargetRef `json:"unassigned"`
}

// RebindResult is the Skill after an explicit Rebind, plus whether the
// rebound Skill converged to in_sync with matching live and Source digests.
type RebindResult struct {
	Skill     Skill `json:"skill"`
	Identical bool  `json:"identical"`
}

func uniqueSortedIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// withSkillCleanupLocks holds the Store exclusive lock, recovers Store
// journals, then locks every Target that still names the Skill — desired
// set, Managed Links, and open link intents — in stable ID order and
// recovers those intents before fn runs.
func (a *App) withSkillCleanupLocks(ctx context.Context, skillID int64, fn func() error) error {
	held, err := a.acquireStoreLock()
	if err != nil {
		return err
	}
	defer held.Unlock()
	if err := a.recoverOpenOperations(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	ids, err := a.skillCleanupTargetIDs(skillID)
	if err != nil {
		return err
	}
	return a.withTargetLocks(ids, fn)
}

// skillCleanupTargetIDs is the lock-order set for one Skill delete.
func (a *App) skillCleanupTargetIDs(skillID int64) ([]int64, error) {
	var ids []int64
	links, err := state.ListManagedLinksBySkill(a.db, skillID)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing Managed Links of Skill %d: %v", skillID, err)
	}
	for _, l := range links {
		ids = append(ids, l.TargetID)
	}
	impact, err := state.GetSkillImpact(a.db, skillID)
	if err != nil {
		return nil, Errorf(CodeInternal, "reading the delete impact of Skill %d: %v", skillID, err)
	}
	for _, t := range impact.Targets {
		ids = append(ids, t.ID)
	}
	intents, err := state.ListOpenLinkIntentsBySkill(a.db, skillID)
	if err != nil {
		return nil, Errorf(CodeInternal, "listing link intents of Skill %d: %v", skillID, err)
	}
	for _, it := range intents {
		ids = append(ids, it.TargetID)
	}
	return uniqueSortedIDs(ids), nil
}

func (a *App) withTargetLocks(ids []int64, fn func() error) error {
	var held []*lock.Lock
	for _, id := range ids {
		l, err := a.acquireTargetLock(id)
		if err != nil {
			for i := len(held) - 1; i >= 0; i-- {
				held[i].Unlock()
			}
			return err
		}
		held = append(held, l)
	}
	defer func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Unlock()
		}
	}()
	for _, id := range ids {
		if err := a.resolveTargetIntents(id); err != nil {
			return err
		}
	}
	return fn()
}

func cleanupLinksFrom(refs []state.ManagedLinkRef) []CleanupLink {
	out := make([]CleanupLink, 0, len(refs))
	for _, r := range refs {
		out = append(out, CleanupLink{
			TargetID: r.TargetID, TargetName: r.TargetName,
			SkillID: r.SkillID, Slug: r.Slug,
		})
	}
	return out
}
