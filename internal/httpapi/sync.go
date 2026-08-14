package httpapi

import (
	"net/http"
	"time"

	"skillctl/internal/app"
	"skillctl/internal/sync"
)

// lastSyncJSON is the latest synchronization outcome on a Skill resource.
type lastSyncJSON struct {
	Action       string `json:"action"`
	Result       string `json:"result"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	BeforeDigest string `json:"before_digest,omitempty"`
	AfterDigest  string `json:"after_digest,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Error        string `json:"error,omitempty"`
}

func newLastSyncJSON(l *app.LastSync) *lastSyncJSON {
	if l == nil {
		return nil
	}
	out := &lastSyncJSON{
		Action: l.Action, Result: l.Result, BeforeDigest: l.BeforeDigest,
		AfterDigest: l.AfterDigest, Revision: l.Revision, Error: l.Error,
	}
	if l.StartedAt != nil {
		out.StartedAt = l.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if l.CompletedAt != nil {
		out.CompletedAt = l.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

// syncItemResultJSON is one synchronization item's stable outcome; failed,
// blocked, or skipped items carry the detail they have.
type syncItemResultJSON struct {
	SkillID      int64  `json:"skill_id"`
	Slug         string `json:"slug"`
	Status       string `json:"status"`
	Stale        bool   `json:"stale"`
	Action       string `json:"action"`
	Result       string `json:"result"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message,omitempty"`
	BeforeDigest string `json:"before_digest,omitempty"`
	AfterDigest  string `json:"after_digest,omitempty"`
	Revision     string `json:"revision,omitempty"`
}

func newSyncItemResultJSON(r *app.SyncItemResult) syncItemResultJSON {
	return syncItemResultJSON{
		SkillID: r.SkillID, Slug: r.Slug, Status: string(r.Status), Stale: r.Stale,
		Action: r.Action, Result: r.Result, Code: r.ErrorCode, Message: r.ErrorMessage,
		BeforeDigest: r.BeforeDigest, AfterDigest: r.AfterDigest, Revision: r.Revision,
	}
}

// syncSummaryJSON tallies the per-item results of one batch.
type syncSummaryJSON struct {
	Total          int `json:"total"`
	NoOp           int `json:"no_op"`
	Updated        int `json:"updated"`
	KeptStore      int `json:"kept_store"`
	AcceptedSource int `json:"accepted_source"`
	Skipped        int `json:"skipped"`
	Blocked        int `json:"blocked"`
	Failed         int `json:"failed"`
	RolledBack     int `json:"rolled_back"`
}

// syncBatchResultJSON is the complete outcome of one synchronization batch.
type syncBatchResultJSON struct {
	Items   []syncItemResultJSON `json:"items"`
	Summary syncSummaryJSON      `json:"summary"`
}

func newSyncBatchResultJSON(r *app.SyncSkillsResult) syncBatchResultJSON {
	out := syncBatchResultJSON{
		Items: make([]syncItemResultJSON, 0, len(r.Items)),
		Summary: syncSummaryJSON{
			Total: r.Summary.Total, NoOp: r.Summary.NoOp, Updated: r.Summary.Updated,
			KeptStore: r.Summary.KeptStore, AcceptedSource: r.Summary.AcceptedSource,
			Skipped: r.Summary.Skipped, Blocked: r.Summary.Blocked,
			Failed: r.Summary.Failed, RolledBack: r.Summary.RolledBack,
		},
	}
	for i := range r.Items {
		out.Items = append(out.Items, newSyncItemResultJSON(&r.Items[i]))
	}
	return out
}

// diffJSON is the REST shape of the three-way difference. The comparison
// entries travel verbatim from the synchronization domain.
type diffJSON struct {
	SkillID        int64             `json:"skill_id"`
	Slug           string            `json:"slug"`
	SourceDigest   string            `json:"source_digest"`
	StoreDigest    string            `json:"store_digest"`
	BaselineDigest string            `json:"baseline_digest"`
	Comparisons    []sync.Comparison `json:"comparisons"`
}

func newDiffJSON(r *app.DiffResult) diffJSON {
	out := diffJSON{
		SkillID: r.SkillID, Slug: r.Slug, SourceDigest: r.SourceDigest,
		StoreDigest: r.StoreDigest, BaselineDigest: r.BaselineDigest,
		Comparisons: r.Comparisons,
	}
	if out.Comparisons == nil {
		out.Comparisons = []sync.Comparison{}
	}
	return out
}

func (s *Server) handleCheckSkillSync(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	sk, err := a.CheckSkillSync(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSkillJSON(*sk))
}

func (s *Server) handleDiffSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	result, err := a.DiffSkill(r.Context(), id, r.URL.Query().Get("path"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newDiffJSON(result))
}

func (s *Server) handleSyncSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	outcome, err := a.SyncSkill(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncItemResultJSON(outcome))
}

func (s *Server) handleKeepStore(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	outcome, err := a.KeepStore(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncItemResultJSON(outcome))
}

func (s *Server) handleAcceptSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	outcome, err := a.AcceptSource(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncItemResultJSON(outcome))
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	outcome, err := a.Rollback(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncItemResultJSON(outcome))
}

func (s *Server) handleSyncSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	result, err := a.SyncSkills(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncBatchResultJSON(result))
}
