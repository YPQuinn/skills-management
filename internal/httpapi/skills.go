package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skillctl/internal/app"
)

// skillBindingJSON is one Skill's Source Binding in the REST view. It is
// omitted when the Skill has no Binding (possible after a later Detach).
type skillBindingJSON struct {
	SourceID     int64  `json:"source_id"`
	SourceName   string `json:"source_name"`
	RelativeDir  string `json:"relative_dir"`
	Digest       string `json:"digest"`
	SourceCommit string `json:"source_commit,omitempty"`
	ImportedAt   string `json:"imported_at"`
}

// skillJSON is the REST Skill resource: the list item and the detail share
// one shape. Timestamps are UTC RFC3339Nano strings.
type skillJSON struct {
	ID             int64             `json:"id"`
	Slug           string            `json:"slug"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	StoreDigest    string            `json:"store_digest"`
	BaselineDigest string            `json:"baseline_digest"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	Binding        *skillBindingJSON `json:"binding,omitempty"`

	SyncStatus    string        `json:"sync_status"`
	SyncStale     bool          `json:"sync_stale"`
	SyncCheckedAt string        `json:"sync_checked_at,omitempty"`
	LastSync      *lastSyncJSON `json:"last_sync,omitempty"`

	HasPreviousSnapshot bool `json:"has_previous_snapshot"`
}

// skillFrontmatterFieldJSON is one YAML frontmatter field of a Skill
// document, in document order. Non-scalar values are YAML text.
type skillFrontmatterFieldJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// skillContentJSON is the live Skill Store copy of one Skill's SKILL.md.
type skillContentJSON struct {
	SkillID     int64                       `json:"skill_id"`
	Slug        string                      `json:"slug"`
	Path        string                      `json:"path"`
	Frontmatter []skillFrontmatterFieldJSON `json:"frontmatter"`
	Body        string                      `json:"body"`
}

// importSkillSelectorJSON selects one Source Inventory entry by exact
// relative path or by unique Inventory name, with per-entry slug override
// and replace permission. A selector must choose exactly one of
// relative_dir or name; the application validates the slug grammar. Slug
// is a pointer so an explicit empty string survives decoding and is
// rejected by the application, while an omitted slug stays nil. Field
// names are the WebUI-frozen contract.
type importSkillSelectorJSON struct {
	RelativeDir string  `json:"relative_dir"`
	Name        string  `json:"name"`
	Slug        *string `json:"slug"`
	Replace     bool    `json:"replace"`
}

// importSkillsRequest is one batch import: the Source id, either every
// Inventory entry (all) or an explicit selector list, and the batch-level
// allow_large override.
type importSkillsRequest struct {
	SourceID   int64                     `json:"source_id"`
	All        bool                      `json:"all"`
	Selectors  []importSkillSelectorJSON `json:"selectors"`
	AllowLarge bool                      `json:"allow_large"`
}

// importItemResultJSON is one batch item's stable outcome; failed items
// carry code and message, replaced items preview the superseded Skill in
// replaces, and conflict/replaced items preview the affected Groups and
// Targets in impact. Field names are the WebUI-frozen contract.
type importItemResultJSON struct {
	Status        string                  `json:"status"`
	RelativeDir   string                  `json:"relative_dir"`
	RequestedSlug string                  `json:"requested_slug,omitempty"`
	Slug          string                  `json:"slug,omitempty"`
	SkillID       int64                   `json:"skill_id,omitempty"`
	Code          string                  `json:"code,omitempty"`
	Message       string                  `json:"message,omitempty"`
	Replaces      *importReplacesInfoJSON `json:"replaces,omitempty"`
	Impact        *importImpactJSON       `json:"impact,omitempty"`
}

// importImpactGroupJSON is one affected Group in the replace-impact
// preview.
type importImpactGroupJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// importImpactTargetJSON is one affected Target with how the Skill reaches
// it.
type importImpactTargetJSON struct {
	ID     int64                   `json:"id"`
	Name   string                  `json:"name"`
	Direct bool                    `json:"direct"`
	Groups []importImpactGroupJSON `json:"groups,omitempty"`
}

// importImpactJSON previews the Groups and Targets affected by an explicit
// Replace.
type importImpactJSON struct {
	Groups  []importImpactGroupJSON  `json:"groups"`
	Targets []importImpactTargetJSON `json:"targets"`
}

// importReplacesInfoJSON previews the existing managed Skill an explicit
// Replace would supersede, with its id and slug for the replace flow.
type importReplacesInfoJSON struct {
	SkillID int64  `json:"skill_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

// importSummaryJSON tallies the per-item statuses of one batch.
type importSummaryJSON struct {
	Total           int `json:"total"`
	Imported        int `json:"imported"`
	AlreadyImported int `json:"already_imported"`
	SkippedConflict int `json:"skipped_conflict"`
	Replaced        int `json:"replaced"`
	Failed          int `json:"failed"`
}

// importSkillsResultJSON is the complete outcome of one valid batch: every
// item result plus the summary. A batch that began processing returns item
// outcomes rather than a top-level error.
type importSkillsResultJSON struct {
	Items   []importItemResultJSON `json:"items"`
	Summary importSummaryJSON      `json:"summary"`
}

func newSkillJSON(s app.Skill) skillJSON {
	out := skillJSON{
		ID: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description,
		StoreDigest: s.StoreDigest, BaselineDigest: s.BaselineDigest,
		CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  s.UpdatedAt.UTC().Format(time.RFC3339Nano),
		SyncStatus: s.SyncStatus, SyncStale: s.SyncStale, LastSync: newLastSyncJSON(s.LastSync),
		HasPreviousSnapshot: s.HasPreviousSnapshot,
	}
	if s.SyncCheckedAt != nil {
		out.SyncCheckedAt = s.SyncCheckedAt.UTC().Format(time.RFC3339Nano)
	}
	if s.Binding != nil {
		out.Binding = &skillBindingJSON{
			SourceID: s.Binding.SourceID, SourceName: s.Binding.SourceName,
			RelativeDir: s.Binding.RelativeDir, Digest: s.Binding.Digest,
			SourceCommit: s.Binding.SourceCommit,
			ImportedAt:   s.Binding.ImportedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return out
}

func newSkillContentJSON(c *app.SkillContent) skillContentJSON {
	out := skillContentJSON{
		SkillID:     c.SkillID,
		Slug:        c.Slug,
		Path:        c.Path,
		Frontmatter: make([]skillFrontmatterFieldJSON, 0, len(c.Frontmatter)),
		Body:        c.Body,
	}
	for _, f := range c.Frontmatter {
		out.Frontmatter = append(out.Frontmatter, skillFrontmatterFieldJSON{Key: f.Key, Value: f.Value})
	}
	return out
}

func newImportSkillsResultJSON(r *app.ImportSkillsResult) importSkillsResultJSON {
	out := importSkillsResultJSON{
		Items: make([]importItemResultJSON, 0, len(r.Items)),
		Summary: importSummaryJSON{
			Total: r.Summary.Total, Imported: r.Summary.Imported,
			AlreadyImported: r.Summary.AlreadyImported, SkippedConflict: r.Summary.SkippedConflict,
			Replaced: r.Summary.Replaced, Failed: r.Summary.Failed,
		},
	}
	for _, it := range r.Items {
		item := importItemResultJSON{
			Status: string(it.Status), RelativeDir: it.RelativeDir,
			RequestedSlug: it.RequestedSlug, Slug: it.Slug, SkillID: it.SkillID,
			Code: it.ErrorCode, Message: it.ErrorMessage,
		}
		if it.Replaces != nil {
			item.Replaces = &importReplacesInfoJSON{
				SkillID: it.Replaces.ID, Slug: it.Replaces.Slug, Name: it.Replaces.Name,
			}
		}
		if it.Impact != nil {
			item.Impact = &importImpactJSON{
				Groups:  make([]importImpactGroupJSON, 0, len(it.Impact.Groups)),
				Targets: make([]importImpactTargetJSON, 0, len(it.Impact.Targets)),
			}
			for _, g := range it.Impact.Groups {
				item.Impact.Groups = append(item.Impact.Groups, importImpactGroupJSON{ID: g.ID, Name: g.Name})
			}
			for _, tg := range it.Impact.Targets {
				t := importImpactTargetJSON{ID: tg.ID, Name: tg.Name, Direct: tg.Direct, Groups: []importImpactGroupJSON{}}
				for _, g := range tg.Groups {
					t.Groups = append(t.Groups, importImpactGroupJSON{ID: g.ID, Name: g.Name})
				}
				item.Impact.Targets = append(item.Impact.Targets, t)
			}
		}
		out.Items = append(out.Items, item)
	}
	return out
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	items, err := a.ListSkills()
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]skillJSON, 0, len(items))
	for _, it := range items {
		out = append(out, newSkillJSON(it))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleShowSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	sk, err := a.ShowSkill(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSkillJSON(*sk))
}

func (s *Server) handleSkillContent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	c, err := a.SkillContent(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSkillContentJSON(c))
}

func (s *Server) handleImportSkills(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	// The request must be exactly one JSON object: a null body decodes
	// into a nil pointer and is rejected as malformed before the
	// application is invoked.
	var req *importSkillsRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	in := app.ImportSkillsInput{SourceID: req.SourceID, All: req.All, AllowLarge: req.AllowLarge}
	for _, sel := range req.Selectors {
		in.Selectors = append(in.Selectors, app.ImportSelector{
			RelativeDir: sel.RelativeDir, Name: sel.Name, Slug: sel.Slug, Replace: sel.Replace,
		})
	}
	result, err := a.ImportSkills(r.Context(), in)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newImportSkillsResultJSON(result))
}

// skillID parses the {id} route parameter; a non-numeric id cannot exist.
func skillID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Skill not found", http.StatusNotFound)
		return 0, false
	}
	return id, true
}
