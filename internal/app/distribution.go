package app

import (
	"errors"
	"fmt"
	"os"
	"time"

	"skillctl/internal/distribution"
	"skillctl/internal/source"
	"skillctl/internal/state"
	"skillctl/internal/target"
)

// Distribution error codes shared by the CLI and REST adapters.
const (
	CodeTargetRedirected = "target_redirected"
	CodeTargetInvalid    = "target_invalid"
	CodeTargetConflict   = "target_conflict"
)

// DistributionStatus is the Distribution Status of one Target: the gate
// state, the last-observed presence of each relation with freshness, and
// the latest reconciliation outcome.
type DistributionStatus struct {
	TargetID        int64                  `json:"target_id"`
	Name            string                 `json:"name"`
	Path            string                 `json:"path"`
	State           string                 `json:"state"`
	StateError      string                 `json:"state_error,omitempty"`
	InspectedAt     *time.Time             `json:"inspected_at"`
	Stale           bool                   `json:"stale"`
	InspectionError string                 `json:"inspection_error,omitempty"`
	Items           []DistributionItemView `json:"items"`
	LastResult      string                 `json:"last_result"`
	LastStartedAt   *time.Time             `json:"last_started_at"`
	LastCompletedAt *time.Time             `json:"last_completed_at"`
	LastError       string                 `json:"last_error,omitempty"`
}

// DistributionItemView is one relation's stored or fresh observation with
// the conflict details: node kind, raw and resolved link targets, the
// expected Store path, and adoption eligibility.
type DistributionItemView struct {
	SkillID        int64  `json:"skill_id"`
	Slug           string `json:"slug"`
	Desired        string `json:"desired"`
	Observed       string `json:"observed"`
	Managed        bool   `json:"managed"`
	Adoptable      bool   `json:"adoptable"`
	NodeKind       string `json:"node_kind,omitempty"`
	RawTarget      string `json:"raw_target,omitempty"`
	ResolvedTarget string `json:"resolved_target,omitempty"`
	ExpectedPath   string `json:"expected_path,omitempty"`
	Stale          bool   `json:"stale"`
	LastResult     string `json:"last_result,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// planItem is one relation's inspection facts feeding the reconciliation
// plan.
type planItem struct {
	skillID    int64
	slug       string
	desired    string
	observed   string
	storeOK    bool
	storeError string
	rawTarget  string
}

// inspectedTarget is one coherent Target inspection: the gate result, the
// physical Store link root, the status view, and the plan facts.
type inspectedTarget struct {
	gate      distribution.Gate
	storeRoot string
	status    *DistributionStatus
	plan      []planItem
}

// storeLinkRoot resolves the normalized physical Store path that link
// targets name (decision 06).
func (a *App) storeLinkRoot() (string, error) {
	opts, err := resolveOptions()
	if err != nil {
		return "", err
	}
	return target.ResolvePhysicalPath(a.StorePath, opts)
}

// storeSkillOK reports whether the physical Store Skill of one slug is a
// valid Skill directory, with the reason it is not.
func storeSkillOK(storeRoot, slug string) (bool, string) {
	dir := distribution.ExpectedPath(storeRoot, slug)
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return false, fmt.Sprintf("the Store Skill %q is missing", slug)
	case err != nil || !info.IsDir():
		return false, fmt.Sprintf("the Store Skill %q is not a directory", slug)
	}
	if err := source.ValidateSkillDir(dir); err != nil {
		return false, fmt.Sprintf("the Store Skill %q is invalid: %v", slug, err)
	}
	return true, ""
}

// fillStoredStatus completes one status view from the stored observation
// after a failed gate or inspection.
func (a *App) fillStoredStatus(out *inspectedTarget, t *state.Target) error {
	items, err := state.ListDistributionItems(a.db, t.ID)
	if err != nil {
		return Errorf(CodeInternal, "reading the stored observation of Target %d: %v", t.ID, err)
	}
	out.status.InspectedAt = t.LastInspectedAt
	out.status.Stale = t.LastInspectedStale
	out.status.InspectionError = t.LastInspectError
	out.status.LastResult = t.LastDistResult
	out.status.LastStartedAt = t.LastDistStartedAt
	out.status.LastCompletedAt = t.LastDistCompleted
	out.status.LastError = t.LastDistError
	out.status.Items = itemViews(items, out.storeRoot)
	return nil
}

// storedDistribution builds the stored Distribution Status view for an
// ordinary Target listing (decision 06): the latest observation with its
// timestamp, without a fresh inspection.
func (a *App) storedDistribution(t *state.Target) (*DistributionStatus, error) {
	storeRoot, err := a.storeLinkRoot()
	if err != nil {
		return nil, err
	}
	out := &inspectedTarget{storeRoot: storeRoot,
		status: &DistributionStatus{TargetID: t.ID, Name: t.Name, Path: t.Path, Items: []DistributionItemView{}}}
	if err := a.fillStoredStatus(out, t); err != nil {
		return nil, err
	}
	return out.status, nil
}

// itemViews converts stored or fresh items into status views, filling the
// expected Store path for desired Skills.
func itemViews(items []state.DistributionItem, storeRoot string) []DistributionItemView {
	out := make([]DistributionItemView, 0, len(items))
	for _, it := range items {
		v := DistributionItemView{
			SkillID: it.SkillID, Slug: it.Slug, Desired: it.Desired, Observed: it.Observed,
			Managed: it.Managed, Adoptable: it.Adoptable, NodeKind: it.NodeKind,
			RawTarget: it.RawTarget, ResolvedTarget: it.ResolvedTarget,
			Stale: it.Stale, LastResult: it.LastResult, LastError: it.LastError,
		}
		if it.Desired == distribution.DesiredPresent {
			v.ExpectedPath = distribution.ExpectedPath(storeRoot, it.Slug)
		}
		out = append(out, v)
	}
	return out
}

// desiredBySlug indexes the desired set by slug.
func desiredBySlug(desired []state.DesiredSkill, slug string) (state.DesiredSkill, bool) {
	for _, d := range desired {
		if d.Skill.Slug == slug {
			return d, true
		}
	}
	return state.DesiredSkill{}, false
}
