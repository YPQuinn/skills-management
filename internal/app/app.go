// Package app is the application boundary: it owns the opened state database
// and Store paths for one initialized installation, and it defines the typed
// errors that the CLI and REST adapters map onto stable codes.
package app

import (
	"context"
	"database/sql"
	"fmt"

	"skillctl/internal/skillstore"
	"skillctl/internal/source"
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

	db       *sql.DB
	observer source.Observer
	store    skillstore.Store

	// materialize is the entry-materialization seam; tests replace it to
	// deterministically cancel or fail one item. Production uses
	// source.MaterializeEntry.
	materialize func(ctx context.Context, loc source.Locator, commit string, entry source.Entry, workDir, dst string, allowLarge bool) (string, error)

	// commitHook, when set, runs immediately before and immediately after
	// each item's SQLite commit; tests use it to cancel the request context
	// at exactly one of the two cancellation boundaries. Nil in production.
	commitHook func(afterCommit bool)

	// deleteOperation is the operation-intent deletion seam; nil means the
	// production state.DeleteOperationIntent (non-terminal rows) or
	// state.DeleteOperationTerminal (terminal receipt rows). Tests replace
	// it to prove that a failed intent deletion after a successful
	// Restore/Finalize or pre-install discard is classified as
	// recovery_failed and blocks later Store writes.
	deleteOperation func(id int64) error

	// receiptPersist is the terminal receipt CAS seam; nil means the
	// production state.MarkOperationFinalized/MarkOperationRestored.
	// Tests replace it to fail the durable receipt CAS after the Store
	// preparation completed, simulating a crash between the semantic
	// terminal mutation and the receipt persistence.
	receiptPersist func(op skillstore.Operation) error
}

// New opens the state database at stateDBPath, resolves every unfinished
// Store operation under the Store exclusive lock, and only then returns
// the App. A proven-unrecoverable operation or Store lock contention
// refuses the open (recovery_failed / locked), so every capability —
// including the read-only show and diff — always sees the recovered Store.
// Application capabilities arrive in later tickets as App methods.
func New(storePath, stateDBPath string) (*App, error) {
	a, err := openApp(storePath, stateDBPath)
	if err != nil {
		return nil, err
	}
	if err := a.recoverOnOpen(); err != nil {
		a.Close()
		return nil, err
	}
	if err := a.recoverLinkIntents(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// openApp opens the state database without resolving open operations. Only
// tests use it: they install seams before the recovery that a later Store
// write resolves, or observe the failure of a recovery they cannot prove.
func openApp(storePath, stateDBPath string) (*App, error) {
	db, err := state.Open(stateDBPath)
	if err != nil {
		return nil, Errorf(CodeInternal, "opening state database: %v", err)
	}
	return &App{
		StorePath: storePath, StateDBPath: stateDBPath, db: db,
		observer: dispatchObserver{}, store: skillstore.New(storePath),
		materialize: source.MaterializeEntry,
	}, nil
}

// Close releases the state database.
func (a *App) Close() error {
	return a.db.Close()
}
