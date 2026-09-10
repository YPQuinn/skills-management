package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skillctl/internal/app"
)

// groupJSON is the REST Group resource. Timestamps are UTC RFC3339Nano
// strings.
type groupJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// groupSummaryJSON is one Group list row.
type groupSummaryJSON struct {
	groupJSON
	MemberCount int `json:"member_count"`
}

// skillRefJSON is the minimal Skill identity for relationship views.
type skillRefJSON struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// groupViewJSON is the Group detail: membership plus assigning Targets.
type groupViewJSON struct {
	groupJSON
	Members []skillRefJSON `json:"members"`
	Targets []targetJSON   `json:"targets"`
}

// addGroupMembersRequest is one membership batch: the Skill ids to add.
type addGroupMembersRequest struct {
	SkillIDs []int64 `json:"skill_ids"`
}

func newGroupJSON(g app.Group) groupJSON {
	return groupJSON{
		ID: g.ID, Name: g.Name,
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: g.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func newGroupViewJSON(g *app.GroupView) groupViewJSON {
	out := groupViewJSON{
		groupJSON: newGroupJSON(g.Group),
		Members:   make([]skillRefJSON, 0, len(g.Members)),
		Targets:   make([]targetJSON, 0, len(g.Targets)),
	}
	for _, m := range g.Members {
		out.Members = append(out.Members, skillRefJSON{ID: m.ID, Slug: m.Slug, Name: m.Name})
	}
	for _, t := range g.Targets {
		out.Targets = append(out.Targets, newTargetJSON(t))
	}
	return out
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	items, err := a.ListGroups()
	if err != nil {
		writeAppError(w, err)
		return
	}
	out := make([]groupSummaryJSON, 0, len(items))
	for _, g := range items {
		out = append(out, groupSummaryJSON{
			groupJSON:   newGroupJSON(g.Group),
			MemberCount: g.MemberCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	var req *createGroupRequest
	if err := decodeJSON(r, &req); err != nil || req == nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	g, err := a.CreateGroup(req.Name)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newGroupJSON(*g))
}

type createGroupRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleShowGroup(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := groupID(w, r)
	if !ok {
		return
	}
	g, err := a.ShowGroup(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newGroupViewJSON(g))
}

func (s *Server) handleAddGroupMembers(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := groupID(w, r)
	if !ok {
		return
	}
	var req *addGroupMembersRequest
	if err := decodeJSON(r, &req); err != nil || req == nil || len(req.SkillIDs) == 0 {
		emitError(w, codeBadRequest, "invalid JSON body: skill_ids must not be empty", http.StatusBadRequest)
		return
	}
	g, err := a.AddGroupSkills(id, req.SkillIDs)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newGroupViewJSON(g))
}

func (s *Server) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := groupID(w, r)
	if !ok {
		return
	}
	skillID, err := strconv.ParseInt(chi.URLParam(r, "skillID"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Group member not found", http.StatusNotFound)
		return
	}
	g, err := a.RemoveGroupSkills(id, []int64{skillID})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newGroupViewJSON(g))
}

// groupID parses the {id} route parameter; a non-numeric id cannot exist.
func groupID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		emitError(w, app.CodeNotFound, "Group not found", http.StatusNotFound)
		return 0, false
	}
	return id, true
}
