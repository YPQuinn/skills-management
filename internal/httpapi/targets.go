package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skillctl/internal/app"
	"skillctl/internal/target"
)

// targetJSON is the REST Target resource. ProjectRoot is present only for
// project-scope Targets.
type targetJSON struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Adapter     string `json:"adapter"`
	Scope       string `json:"scope"`
	ProjectRoot string `json:"project_root,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// assignmentJSON is one Target Assignment with its resolved subject.
type assignmentJSON struct {
	ID        int64         `json:"id"`
	Kind      string        `json:"kind"`
	Skill     *skillRefJSON `json:"skill,omitempty"`
	Group     *groupRefJSON `json:"group,omitempty"`
	CreatedAt string        `json:"created_at"`
}

// groupRefJSON is the minimal Group identity for Assignment views.
type groupRefJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// desireReasonJSON is one contributing Assignment in the desired set.
type desireReasonJSON struct {
	AssignmentID int64  `json:"assignment_id"`
	Kind         string `json:"kind"`
	GroupID      int64  `json:"group_id,omitempty"`
	GroupName    string `json:"group_name,omitempty"`
}

// desiredSkillJSON is one desired Skill with its reasons.
type desiredSkillJSON struct {
	ID      int64              `json:"id"`
	Slug    string             `json:"slug"`
	Name    string             `json:"name"`
	Reasons []desireReasonJSON `json:"reasons"`
}

// targetViewJSON is the Target detail: identity, compatible adapters, its
// direct Skill and Group Assignments, and the expanded desired set.
type targetViewJSON struct {
	targetJSON
	CompatibleAdapters []string                `json:"compatible_adapters,omitempty"`
	DirectSkills       []assignmentJSON        `json:"direct_skills"`
	Groups             []assignmentJSON        `json:"groups"`
	DesiredSkills      []desiredSkillJSON      `json:"desired_skills"`
	Distribution       *distributionStatusJSON `json:"distribution,omitempty"`
}

// createTargetRequest is one Target registration: either a built-in
// adapter with a scope (plus a project root for project scope) or a custom
// path. The application validates the exclusive forms.
type createTargetRequest struct {
	Name        string `json:"name"`
	Adapter     string `json:"adapter"`
	Scope       string `json:"scope"`
	ProjectRoot string `json:"project_root"`
	Path        string `json:"path"`
}

// createAssignmentRequest is one Assignment: exactly one of skill_id or
// group_id, selected by kind.
type createAssignmentRequest struct {
	Kind    string `json:"kind"`
	SkillID int64  `json:"skill_id"`
	GroupID int64  `json:"group_id"`
}

// adapterJSON is one built-in Target Adapter with its on-demand advisory
// detection result.
type adapterJSON struct {
	Key       string        `json:"key"`
	Name      string        `json:"name"`
	Detection detectionJSON `json:"detection"`
}

// detectionJSON is one adapter's advisory detection result.
type detectionJSON struct {
	Status     string   `json:"status"`
	Evidence   []string `json:"evidence"`
	DetectedAt string   `json:"detected_at"`
}

func newTargetJSON(t app.Target) targetJSON {
	return targetJSON{
		ID: t.ID, Name: t.Name, Path: t.Path, Adapter: t.Adapter, Scope: t.Scope,
		ProjectRoot: t.ProjectRoot,
		CreatedAt:   t.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   t.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func newAssignmentJSON(a app.AssignmentView) assignmentJSON {
	out := assignmentJSON{
		ID: a.ID, Kind: a.Kind,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if a.Skill != nil {
		out.Skill = &skillRefJSON{ID: a.Skill.ID, Slug: a.Skill.Slug, Name: a.Skill.Name}
	}
	if a.Group != nil {
		out.Group = &groupRefJSON{ID: a.Group.ID, Name: a.Group.Name}
	}
	return out
}

func newTargetViewJSON(t *app.TargetView) targetViewJSON {
	out := targetViewJSON{
		targetJSON:         newTargetJSON(t.Target),
		CompatibleAdapters: t.CompatibleAdapters,
		DirectSkills:       make([]assignmentJSON, 0, len(t.DirectSkills)),
		Groups:             make([]assignmentJSON, 0, len(t.Groups)),
		DesiredSkills:      make([]desiredSkillJSON, 0, len(t.DesiredSkills)),
	}
	for _, a := range t.DirectSkills {
		out.DirectSkills = append(out.DirectSkills, newAssignmentJSON(a))
	}
	for _, a := range t.Groups {
		out.Groups = append(out.Groups, newAssignmentJSON(a))
	}
	for _, d := range t.DesiredSkills {
		item := desiredSkillJSON{ID: d.ID, Slug: d.Slug, Name: d.Name, Reasons: make([]desireReasonJSON, 0, len(d.Reasons))}
		for _, r := range d.Reasons {
			item.Reasons = append(item.Reasons, desireReasonJSON{
				AssignmentID: r.AssignmentID, Kind: r.Kind, GroupID: r.GroupID, GroupName: r.GroupName,
			})
		}
		out.DesiredSkills = append(out.DesiredSkills, item)
	}
	if t.Distribution != nil {
		d := newDistributionStatusJSON(t.Distribution)
		out.Distribution = &d
	}
	return out
}

// handleListAdapters returns the built-in adapter table with on-demand
// detection. Detection is advisory and read-only, so the endpoint works
// before initialization (the setup page uses it).
func (s *Server) handleListAdapters(w http.ResponseWriter, r *http.Request) {
	detections := target.DetectAll(target.DefaultDetectionOptions())
	out := make([]adapterJSON, 0, len(detections))
	for _, d := range detections {
		a, _ := target.ByKey(d.Adapter)
		out = append(out, adapterJSON{
			Key:  a.Key,
			Name: a.Name,
			Detection: detectionJSON{
				Status:     string(d.Status),
				Evidence:   d.Evidence,
				DetectedAt: d.DetectedAt.UTC().Format(time.RFC3339Nano),
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	items, err := a.ListTargets()
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]targetJSON, 0, len(items))
	for _, t := range items {
		out = append(out, newTargetJSON(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleRegisterTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	var req *createTargetRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	view, err := a.RegisterTarget(app.TargetInput{
		Name: req.Name, Adapter: req.Adapter, Scope: req.Scope,
		ProjectRoot: req.ProjectRoot, Path: req.Path,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTargetViewJSON(view))
}

func (s *Server) handleShowTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	view, err := a.ShowTarget(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTargetViewJSON(view))
}

func (s *Server) handleCreateAssignment(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	var req *createAssignmentRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	as, err := a.AssignTarget(id, app.AssignInput{Kind: req.Kind, SkillID: req.SkillID, GroupID: req.GroupID})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newAssignmentJSON(*as))
}

func (s *Server) handleDeleteAssignment(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	assignmentID, err := strconv.ParseInt(chi.URLParam(r, "assignmentID"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Assignment not found", http.StatusNotFound)
		return
	}
	view, err := a.UnassignTargetByID(id, assignmentID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTargetViewJSON(view))
}

// targetID parses the {id} route parameter; a non-numeric id cannot exist.
func targetID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Target not found", http.StatusNotFound)
		return 0, false
	}
	return id, true
}
