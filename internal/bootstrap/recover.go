package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"skillctl/internal/app"
	"skillctl/internal/lock"
)

// RecoverStore rebuilds a missing state database from the configured Skill
// Store. It refuses every installation that is not state_missing, never
// rewrites the configuration or Store content, and transitions the
// in-process application the same way Initialize does.
func (m *Manager) RecoverStore() (*app.StoreRecoveryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.detectStateMissingLocked(); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(m.baseDir(), 0o755); err != nil {
		return nil, app.Errorf(app.CodeInternal, "creating %s: %v", m.baseDir(), err)
	}
	writer, err := lock.TryExclusive(filepath.Join(m.baseDir(), "skillctl.lock"))
	if errors.Is(err, lock.ErrLocked) {
		return nil, app.Errorf(app.CodeLocked, "another skillctl process is already modifying the installation")
	}
	if err != nil {
		return nil, app.Errorf(app.CodeInternal, "acquiring writer lock: %v", err)
	}
	defer writer.Unlock()

	if err := m.detectStateMissingLocked(); err != nil {
		return nil, err
	}

	// Close any cached App before creating a replacement state.db so a
	// long-lived request cannot keep writing through a handle to the
	// deleted inode.
	m.detachAppLocked()

	cfg, err := readConfig(m.configPath)
	if err != nil {
		return nil, app.Errorf(app.CodeInvalidConfig, "invalid configuration: %v", err)
	}
	if info, err := os.Lstat(cfg.StorePath); err != nil {
		return nil, app.Errorf(app.CodeInternal, "Skill Store %s is not readable: %v", cfg.StorePath, err)
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, app.Errorf(app.CodeInvalidConfig, "Skill Store %s is not a directory", cfg.StorePath)
	}

	a, err := app.New(cfg.StorePath, cfg.StateDBPath)
	if err != nil {
		return nil, err
	}
	result, err := a.RecoverLostStore(context.Background())
	if err != nil {
		a.Close()
		removeStateDB(cfg.StateDBPath)
		return nil, err
	}
	m.app = a
	return result, nil
}

func (m *Manager) detectStateMissingLocked() error {
	st, err := m.detectLocked()
	if err != nil {
		return err
	}
	switch st {
	case StateMissing:
		return nil
	case StateReady:
		return app.Errorf(app.CodeAlreadyInitialized, "Skill Manager is already initialized")
	case StateUninitialized:
		return app.Errorf(app.CodeNotInitialized, "Skill Manager is not initialized; run init without --recover-store")
	}
	return app.Errorf(app.CodeInvalidConfig, "installation is not recoverable")
}

func removeStateDB(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
}
