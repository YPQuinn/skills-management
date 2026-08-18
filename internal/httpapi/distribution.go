package httpapi

import (
	"net/http"
	"time"

	"skillctl/internal/app"
)

// distributionItemJSON is one relation's Distribution Status observation
// with its conflict details.
type distributionItemJSON struct {
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

// distributionStatusJSON is the REST Distribution Status view.
type distributionStatusJSON struct {
	TargetID        int64                  `json:"target_id"`
	Name            string                 `json:"name"`
	Path            string                 `json:"path"`
	State           string                 `json:"state"`
	StateError      string                 `json:"state_error,omitempty"`
	InspectedAt     string                 `json:"inspected_at,omitempty"`
	Stale           bool                   `json:"stale"`
	InspectionError string                 `json:"inspection_error,omitempty"`
	Items           []distributionItemJSON `json:"items"`
	LastResult      string                 `json:"last_result"`
	LastStartedAt   string                 `json:"last_started_at,omitempty"`
	LastCompletedAt string                 `json:"last_completed_at,omitempty"`
	LastError       string                 `json:"last_error,omitempty"`
}

func newDistributionStatusJSON(s *app.DistributionStatus) distributionStatusJSON {
	out := distributionStatusJSON{
		TargetID: s.TargetID, Name: s.Name, Path: s.Path,
		State: s.State, StateError: s.StateError,
		Stale: s.Stale, InspectionError: s.InspectionError,
		Items:      make([]distributionItemJSON, 0, len(s.Items)),
		LastResult: s.LastResult, LastError: s.LastError,
	}
	if s.InspectedAt != nil {
		out.InspectedAt = s.InspectedAt.UTC().Format(time.RFC3339Nano)
	}
	if s.LastStartedAt != nil {
		out.LastStartedAt = s.LastStartedAt.UTC().Format(time.RFC3339Nano)
	}
	if s.LastCompletedAt != nil {
		out.LastCompletedAt = s.LastCompletedAt.UTC().Format(time.RFC3339Nano)
	}
	for _, it := range s.Items {
		out.Items = append(out.Items, distributionItemJSON{
			SkillID: it.SkillID, Slug: it.Slug, Desired: it.Desired, Observed: it.Observed,
			Managed: it.Managed, Adoptable: it.Adoptable, NodeKind: it.NodeKind,
			RawTarget: it.RawTarget, ResolvedTarget: it.ResolvedTarget,
			ExpectedPath: it.ExpectedPath, Stale: it.Stale,
			LastResult: it.LastResult, LastError: it.LastError,
		})
	}
	return out
}

// distributionItemResultJSON is one item's reconciliation result.
type distributionItemResultJSON struct {
	SkillID  int64  `json:"skill_id"`
	Slug     string `json:"slug"`
	Desired  string `json:"desired"`
	Observed string `json:"observed"`
	Action   string `json:"action"`
	Result   string `json:"result"`
	Error    string `json:"error,omitempty"`
}

// distributionSummaryJSON tallies the per-item results.
type distributionSummaryJSON struct {
	Total           int `json:"total"`
	NoOp            int `json:"no_op"`
	Created         int `json:"created"`
	Removed         int `json:"removed"`
	Adopted         int `json:"adopted"`
	BlockedConflict int `json:"blocked_conflict"`
	BlockedBroken   int `json:"blocked_broken"`
	OwnershipLost   int `json:"ownership_lost"`
	Failed          int `json:"failed"`
}

// distributionResultJSON is the complete reconciliation outcome.
type distributionResultJSON struct {
	TargetID  int64                        `json:"target_id"`
	DryRun    bool                         `json:"dry_run"`
	Outcome   string                       `json:"outcome"`
	Error     string                       `json:"error,omitempty"`
	Inspected string                       `json:"inspected_at,omitempty"`
	Stale     bool                         `json:"stale"`
	Items     []distributionItemResultJSON `json:"items"`
	Summary   distributionSummaryJSON      `json:"summary"`
}

func newDistributionResultJSON(r *app.DistributionResult) distributionResultJSON {
	out := distributionResultJSON{
		TargetID: r.TargetID, DryRun: r.DryRun, Outcome: r.Outcome,
		Error: r.Error, Stale: r.Stale,
		Items: make([]distributionItemResultJSON, 0, len(r.Items)),
		Summary: distributionSummaryJSON{
			Total: r.Summary.Total, NoOp: r.Summary.NoOp, Created: r.Summary.Created,
			Removed: r.Summary.Removed, Adopted: r.Summary.Adopted,
			BlockedConflict: r.Summary.BlockedConflict, BlockedBroken: r.Summary.BlockedBroken,
			OwnershipLost: r.Summary.OwnershipLost, Failed: r.Summary.Failed,
		},
	}
	if r.Inspected != nil {
		out.Inspected = r.Inspected.UTC().Format(time.RFC3339Nano)
	}
	for i := range r.Items {
		it := &r.Items[i]
		out.Items = append(out.Items, distributionItemResultJSON{
			SkillID: it.SkillID, Slug: it.Slug, Desired: it.Desired, Observed: it.Observed,
			Action: it.Action, Result: it.Result, Error: it.Error,
		})
	}
	return out
}

// adoptResultJSON is the REST adoption outcome.
type adoptResultJSON struct {
	TargetID  int64  `json:"target_id"`
	SkillID   int64  `json:"skill_id"`
	Slug      string `json:"slug"`
	LinkPath  string `json:"link_path"`
	RawTarget string `json:"raw_target"`
	AdoptedAt string `json:"adopted_at"`
	Result    string `json:"result"`
}

func newAdoptResultJSON(r *app.AdoptResult) adoptResultJSON {
	return adoptResultJSON{
		TargetID: r.TargetID, SkillID: r.SkillID, Slug: r.Slug,
		LinkPath: r.LinkPath, RawTarget: r.RawTarget,
		AdoptedAt: r.AdoptedAt.UTC().Format(time.RFC3339Nano),
		Result:    r.Result,
	}
}

// distributeRequest is one reconciliation request; dry_run selects the
// plan-only mode.
type distributeRequest struct {
	DryRun bool `json:"dry_run"`
}

// adoptRequest names the Skill whose existing link is adopted.
type adoptRequest struct {
	SkillID int64 `json:"skill_id"`
}

// handleInspectTarget runs one fresh coherent Target inspection.
func (s *Server) handleInspectTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	st, err := a.InspectTarget(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newDistributionStatusJSON(st))
}

// handleDistributeTarget reconciles one Target with its Assignments; a
// valid request always returns 200 with the complete per-item outcomes.
func (s *Server) handleDistributeTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	var req *distributeRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.DistributeTarget(r.Context(), id, req.DryRun)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newDistributionResultJSON(res))
}

// handleAdoptTargetLink explicitly adopts one eligible existing symlink.
func (s *Server) handleAdoptTargetLink(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	var req *adoptRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.SkillID == 0 {
		emitError(w, app.CodeInvalidArgument, "adopt requires a skill_id", http.StatusBadRequest)
		return
	}
	res, err := a.AdoptTargetLink(r.Context(), id, req.SkillID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newAdoptResultJSON(res))
}
