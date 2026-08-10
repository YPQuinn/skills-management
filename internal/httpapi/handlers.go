package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

type statusResponse struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type setupRequest struct {
	StorePath string `json:"store_path"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.bm.Detect()
	resp := statusResponse{State: string(st)}
	if err != nil {
		resp.Message = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSetup runs the shared Initialize use case and transitions the
// running manager without a restart.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		emitError(w, codeBadRequest, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.StorePath != "" && !filepath.IsAbs(req.StorePath) {
		emitError(w, app.CodeInvalidArgument, "store_path must be an absolute path", http.StatusBadRequest)
		return
	}

	st, err := s.bm.Detect()
	if err != nil {
		writeAppError(w, err)
		return
	}
	switch st {
	case bootstrap.StateReady:
		emitError(w, app.CodeAlreadyInitialized, "Skill Manager is already initialized", http.StatusConflict)
		return
	case bootstrap.StateMissing:
		emitError(w, app.CodeStateMissing, "state database is missing; fix or remove the configuration to reinitialize", http.StatusConflict)
		return
	}

	if err := s.bm.Initialize(req.StorePath); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{State: string(bootstrap.StateReady)})
}

func handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	emitError(w, codeNotFound, "API route not found", http.StatusNotFound)
}

// decodeJSON decodes exactly one JSON value and rejects unknown fields.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after JSON body")
	}
	return nil
}
