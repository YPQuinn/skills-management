package httpapi

import (
	"net/http"
)

type deleteSourceRequest struct {
	DetachSkills bool `json:"detach_skills"`
}

type deleteSkillRequest struct {
	Cleanup bool `json:"cleanup"`
}

type deleteGroupRequest struct {
	Unassign bool `json:"unassign"`
}

type rebindSkillRequest struct {
	SourceID    int64  `json:"source_id"`
	RelativeDir string `json:"relative_dir"`
	Name        string `json:"name"`
}

func (s *Server) handlePreviewDeleteSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	preview, err := a.PreviewDeleteSource(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var req deleteSourceRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.DeleteSource(r.Context(), id, req.DetachSkills)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDetachSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	sk, err := a.DetachSkill(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newSkillJSON(*sk))
}

func (s *Server) handleRebindSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	var req rebindSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.RebindSkill(r.Context(), id, req.SourceID, req.RelativeDir, req.Name)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePreviewDeleteSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	preview, err := a.PreviewDeleteSkill(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := skillID(w, r)
	if !ok {
		return
	}
	var req deleteSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.DeleteSkill(r.Context(), id, req.Cleanup)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePreviewDeleteTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	preview, err := a.PreviewDeleteTarget(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := targetID(w, r)
	if !ok {
		return
	}
	if err := decodeEmptyObject(r); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.DeleteTarget(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePreviewDeleteGroup(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := groupID(w, r)
	if !ok {
		return
	}
	preview, err := a.PreviewDeleteGroup(id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	a, ok := s.app(w, r)
	if !ok {
		return
	}
	id, ok := groupID(w, r)
	if !ok {
		return
	}
	var req deleteGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	res, err := a.DeleteGroup(r.Context(), id, req.Unassign)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
