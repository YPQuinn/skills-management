// Package app is the application boundary: it owns the opened state database
// and Store paths for one initialized installation, and it defines the typed
// errors that the CLI and REST adapters map onto stable codes.
package app

import (
	"database/sql"
	"fmt"

	"skillctl/internal/state"
)

// Stable error codes shared by the CLI and REST adapters.
const (
	CodeInvalidArgument    = "invalid_argument"
	CodeInvalidConfig      = "invalid_config"
	CodeNotInitialized     = "not_initialized"
	CodeAlreadyInitialized = "already_initialized"
	CodeStateMissing       = "state_missing"
	CodeLocked             = "locked"
	CodeInternal           = "internal"
)

// Error is a typed application error with a stable machine-readable code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Errorf builds a typed Error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// App is the in-process application for one initialized installation.
type App struct {
	StorePath   string
	StateDBPath string

	db *sql.DB
}

// New opens the state database at stateDBPath and returns the App bound to
// it. Application capabilities arrive in later tickets as App methods.
func New(storePath, stateDBPath string) (*App, error) {
	db, err := state.Open(stateDBPath)
	if err != nil {
		return nil, Errorf(CodeInternal, "opening state database: %v", err)
	}
	return &App{StorePath: storePath, StateDBPath: stateDBPath, db: db}, nil
}

// Close releases the state database.
func (a *App) Close() error {
	return a.db.Close()
}
