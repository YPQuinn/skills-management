package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skillctl/internal/app"
	"skillctl/internal/source"
)

type sourceSummaryJSON struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	Location              string `json:"location"`
	Ref                   string `json:"ref,omitempty"`
	Subpath               string `json:"subpath,omitempty"`
	Available             bool   `json:"available"`
	Stale                 bool   `json:"stale"`
	LastError             string `json:"last_error,omitempty"`
	LastCheckStartedAt    string `json:"last_check_started_at,omitempty"`
	LastCheckedAt         string `json:"last_checked_at,omitempty"`
	LastCheckResult       string `json:"last_check_result,omitempty"`
	LastSuccessfulCheckAt string `json:"last_successful_check_at,omitempty"`
	LastCommit            string `json:"last_commit,omitempty"`
	LastInventoryDigest   string `json:"last_inventory_digest,omitempty"`
	EntryCount            int    `json:"entry_count"`
}

type sourceEntryJSON struct {
	RelativeDir string `json:"relative_dir"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Digest      string `json:"digest,omitempty"`
}

type sourceIssueJSON struct {
	RelativeDir string `json:"relative_dir"`
	Reason      string `json:"reason"`
}

type sourceJSON struct {
	sourceSummaryJSON
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Inventory []sourceEntryJSON `json:"inventory"`
	Issues    []sourceIssueJSON `json:"issues"`
}

type createSourceRequest struct {
	Kind     string `json:"kind"`
	Location string `json:"location"`
	Ref      string `json:"ref"`
	Subpath  string `json:"subpath"`
	Name     string `json:"name"`
}

// checkSourceRequest is intentionally empty: a check takes no parameters,
// but the strict decoder still requires exactly one JSON object.
type checkSourceRequest struct{}

func newSourceSummaryJSON(s source.Summary) sourceSummaryJSON {
	return sourceSummaryJSON{
		ID: s.ID, Name: s.Name, Kind: string(s.Kind), Location: s.Location,
		Ref: s.Ref, Subpath: s.Subpath, Available: s.Available, Stale: s.Stale,
		LastError: s.LastError, LastCommit: s.LastCommit,
		LastCheckResult:       s.LastCheckResult,
		LastInventoryDigest:   s.LastInventoryDigest,
		EntryCount:            s.EntryCount,
		LastCheckStartedAt:    formatTime(s.LastCheckStartedAt),
		LastCheckedAt:         formatTime(s.LastCheckedAt),
		LastSuccessfulCheckAt: formatTime(s.LastSuccessfulCheckAt),
	}
}

func newSourceJSON(s *source.Source) sourceJSON {
	out := sourceJSON{
		sourceSummaryJSON: sourceSummaryJSON{
			ID: s.ID, Name: s.Name, Kind: string(s.Kind), Location: s.Location,
			Ref: s.Ref, Subpath: s.Subpath, Available: s.Available, Stale: !s.Available,
			LastError: s.LastError, LastCommit: s.LastCommit,
			LastCheckResult:       s.LastCheckResult,
			LastInventoryDigest:   s.LastInventoryDigest,
			EntryCount:            len(s.Entries),
			LastCheckStartedAt:    formatTime(s.LastCheckStartedAt),
			LastCheckedAt:         formatTime(s.LastCheckedAt),
			LastSuccessfulCheckAt: formatTime(s.LastSuccessfulCheckAt),
		},
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: s.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Inventory: []sourceEntryJSON{},
		Issues:    []sourceIssueJSON{},
	}
	for _, e := range s.Entries {
		out.Inventory = append(out.Inventory, sourceEntryJSON{RelativeDir: e.RelativeDir, Name: e.Name, Description: e.Description, Digest: e.Digest})
	}
	for _, i := range s.Issues {
		out.Issues = append(out.Issues, sourceIssueJSON{RelativeDir: i.RelativeDir, Reason: i.Reason})
	}
	return out
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// app returns the in-process application, or reports a stable error.
func (s *Server) app(w http.ResponseWriter, r *http.Request) (*app.App, bool) {
	a, err := s.bm.App()
	if err != nil {
		writeAppError(w, err)
		return nil, false
	}
	return a, true
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	items, err := a.ListSources()
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]sourceSummaryJSON, 0, len(items))
	for _, it := range items {
		out = append(out, newSourceSummaryJSON(it))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	var req createSourceRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	created, err := a.AddSource(r.Context(), source.AddInput{
		Kind:     source.Kind(req.Kind),
		Location: req.Location,
		Ref:      req.Ref,
		Subpath:  req.Subpath,
		Name:     req.Name,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newSourceJSON(created))
}

func (s *Server) handleShowSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	src, err := a.ShowSource(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSourceJSON(src))
}

func (s *Server) handleCheckSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	// The body must be exactly one JSON object, decoded strictly, before
	// anything is mutated: malformed, trailing, unknown, or non-object
	// input is a bad request and cannot advance check state.
	var req *checkSourceRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	src, err := a.CheckSource(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSourceJSON(src))
}

// sourceID parses the {id} route parameter; a non-numeric id cannot exist.
func sourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Source not found", http.StatusNotFound)
		return 0, false
	}
	return id, true
}
