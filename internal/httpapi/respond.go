package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"skillctl/internal/app"
)

// errorEnvelope is the stable REST error shape from decision 08. encoding/json
// escapes HTML in the message by default, so error text cannot inject markup.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// emitError writes a stable JSON error envelope with details as an empty
// object (never null).
func emitError(w http.ResponseWriter, code, message string, status int) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message, Details: map[string]any{}}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeAppError maps a typed application error onto the stable REST status
// and error envelope.
func writeAppError(w http.ResponseWriter, err error) {
	var ae *app.Error
	if !errors.As(err, &ae) {
		emitError(w, app.CodeInternal, err.Error(), http.StatusInternalServerError)
		return
	}
	switch ae.Code {
	case app.CodeInvalidArgument:
		emitError(w, ae.Code, ae.Message, http.StatusBadRequest)
	case app.CodeNotFound:
		emitError(w, ae.Code, ae.Message, http.StatusNotFound)
	case app.CodeLocked, app.CodeNotInitialized, app.CodeAlreadyInitialized, app.CodeStateMissing, app.CodeInvalidConfig, app.CodeConflict, app.CodeSourceUnavailable:
		emitError(w, ae.Code, ae.Message, http.StatusConflict)
	default:
		emitError(w, ae.Code, ae.Message, http.StatusInternalServerError)
	}
}
