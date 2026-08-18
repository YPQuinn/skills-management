// Package bootstrap owns the uninitialized/ready transition: it detects the
// installation state from disk, parses the configuration, and provides the
// single Initialize use case shared by `skillctl init` and the REST setup
// endpoint. Initialization transitions the in-process application without a
// restart.
package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"skillctl/internal/app"
	"skillctl/internal/lock"
	"skillctl/internal/state"
)

// State is the detected installation state.
type State string

const (
	StateUninitialized State = "uninitialized"
	StateMissing       State = "state_missing"
	StateInvalid       State = "invalid"
	StateReady         State = "ready"
)

// Manager tracks one installation rooted at configPath
// (<base>/config.toml). The default Store, state database, and lock file
// live in the same base directory.
type Manager struct {
	mu         sync.RWMutex
	configPath string
	app        *app.App
}

// New returns a Manager for the installation whose configuration lives at
// configPath.
func New(configPath string) *Manager {
	return &Manager{configPath: configPath}
}

func (m *Manager) baseDir() string {
	return filepath.Dir(m.configPath)
}

// Detect re-checks the installation from disk: no configuration means
// uninitialized; an unparseable or invalid configuration reports StateInvalid
// with the reason; a valid configuration without its state database reports
// StateMissing; an existing state database whose schema is newer than this
// executable or is corrupt reports StateInvalid; otherwise the installation
// is ready.
func (m *Manager) Detect() (State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.detectLocked()
}

func (m *Manager) detectLocked() (State, error) {
	cfg, err := readConfig(m.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StateUninitialized, nil
		}
		return StateInvalid, app.Errorf(app.CodeInvalidConfig, "invalid configuration: %v", err)
	}
	if _, err := os.Stat(cfg.StateDBPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StateMissing, nil
		}
		return StateInvalid, app.Errorf(app.CodeInvalidConfig, "state database at %s is not readable: %v", cfg.StateDBPath, err)
	}
	// The schema is inspected read-only: Detect never migrates, and a newer
	// or corrupt schema is invalid, not ready.
	if _, err := state.InspectSchema(cfg.StateDBPath); err != nil {
		return StateInvalid, app.Errorf(app.CodeInvalidConfig, "state database at %s is not valid: %v", cfg.StateDBPath, err)
	}
	return StateReady, nil
}

// Config returns the parsed configuration, or a typed error when the
// installation is uninitialized or the configuration is invalid.
func (m *Manager) Config() (Config, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, err := readConfig(m.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, app.Errorf(app.CodeNotInitialized, "Skill Manager is not initialized")
		}
		return Config{}, app.Errorf(app.CodeInvalidConfig, "invalid configuration: %v", err)
	}
	return cfg, nil
}

// App returns the in-process application, opening and caching it on first
// use. It refuses to create missing state: without the state database it
// reports state_missing even when an earlier open is cached.
func (m *Manager) App() (*app.App, error) {
	cfg, err := m.Config()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cfg.StateDBPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.mu.Lock()
			m.detachAppLocked()
			m.mu.Unlock()
			return nil, app.Errorf(app.CodeStateMissing, "state database is missing")
		}
		return nil, app.Errorf(app.CodeInvalidConfig, "state database at %s is not readable: %v", cfg.StateDBPath, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.app != nil {
		return m.app, nil
	}
	a, err := app.New(cfg.StorePath, cfg.StateDBPath)
	if err != nil {
		return nil, err
	}
	m.app = a
	return a, nil
}

// detachAppLocked closes and forgets a cached App so later requests cannot
// keep using a handle whose state.db was deleted. The caller holds m.mu.
func (m *Manager) detachAppLocked() {
	if m.app == nil {
		return
	}
	_ = m.app.Close()
	m.app = nil
}

// Initialize is the shared init use case used by `skillctl init` and the
// REST setup endpoint. It refuses already-initialized, state_missing, and
// invalid installations, then creates the Store and state database and
// writes the configuration under the cross-process writer lock.
func (m *Manager) Initialize(storePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	store := storePath
	if store == "" {
		store = filepath.Join(m.baseDir(), "store")
	}
	if !filepath.IsAbs(store) {
		return app.Errorf(app.CodeInvalidArgument, "Store path must be absolute: %s", store)
	}

	// Fast path: refuse already-initialized, state_missing, and invalid
	// installations before touching the writer lock.
	if err := m.detectUninitializedLocked(); err != nil {
		return err
	}

	dbPath := filepath.Join(m.baseDir(), "state.db")

	if err := os.MkdirAll(m.baseDir(), 0o755); err != nil {
		return app.Errorf(app.CodeInternal, "creating %s: %v", m.baseDir(), err)
	}

	writer, err := lock.TryExclusive(filepath.Join(m.baseDir(), "skillctl.lock"))
	if errors.Is(err, lock.ErrLocked) {
		return app.Errorf(app.CodeLocked, "another skillctl process is already modifying the installation")
	}
	if err != nil {
		return app.Errorf(app.CodeInternal, "acquiring writer lock: %v", err)
	}
	defer writer.Unlock()

	// Re-read config and state under the writer lock: another process may
	// have completed initialization while we were waiting, and its
	// configuration and state must never be rewritten.
	if err := m.detectUninitializedLocked(); err != nil {
		return err
	}

	if err := os.MkdirAll(store, 0o755); err != nil {
		return app.Errorf(app.CodeInternal, "creating Store %s: %v", store, err)
	}

	a, err := app.New(store, dbPath)
	if err != nil {
		return err
	}
	if err := writeConfigAtomic(m.configPath, Config{
		Version:     ConfigFormatVersion,
		StorePath:   store,
		StateDBPath: dbPath,
	}); err != nil {
		a.Close()
		return app.Errorf(app.CodeInternal, "writing configuration: %v", err)
	}

	m.app = a
	return nil
}

// detectUninitializedLocked returns nil only when the installation is still
// uninitialized; ready, state_missing, and invalid installations are refused
// with their stable errors. It must run under the cross-process writer lock
// before any mutation so a concurrent process cannot be overwritten.
func (m *Manager) detectUninitializedLocked() error {
	st, err := m.detectLocked()
	if err != nil {
		return err
	}
	switch st {
	case StateReady:
		return app.Errorf(app.CodeAlreadyInitialized, "Skill Manager is already initialized")
	case StateMissing:
		return app.Errorf(app.CodeStateMissing, "state database is missing; fix or remove the configuration to reinitialize")
	}
	return nil
}
