package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
	"skillctl/internal/lock"
	"skillctl/internal/state"
)

func cfgPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".skillctl", "config.toml")
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var ae *app.Error
	if !errors.As(err, &ae) {
		t.Fatalf("got %v, want app.Error with code %s", err, code)
	}
	if ae.Code != code {
		t.Fatalf("error code: got %q, want %q (%v)", ae.Code, code, ae.Message)
	}
}

func TestDetectUninitialized(t *testing.T) {
	st, err := New(cfgPath(t)).Detect()
	if err != nil {
		t.Fatal(err)
	}
	if st != StateUninitialized {
		t.Fatalf("got %q, want %q", st, StateUninitialized)
	}
}

func TestInitializeDefaultAndCustomStore(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	if st, err := m.Detect(); err != nil || st != StateReady {
		t.Fatalf("after init: state %q, err %v", st, err)
	}
	base := filepath.Dir(path)
	for _, want := range []string{filepath.Join(base, "store"), filepath.Join(base, "state.db")} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("%s missing: %v", want, err)
		}
	}
	cfg, err := m.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != ConfigFormatVersion || cfg.StorePath != filepath.Join(base, "store") || cfg.StateDBPath != filepath.Join(base, "state.db") {
		t.Fatalf("config = %+v", cfg)
	}

	// the application is live in-process right after Initialize
	a, err := m.App()
	if err != nil {
		t.Fatal(err)
	}
	if a.StorePath != cfg.StorePath || a.StateDBPath != cfg.StateDBPath {
		t.Fatalf("app paths: %+v", a)
	}

	// custom absolute Store path
	path2 := cfgPath(t)
	m2 := New(path2)
	custom := filepath.Join(t.TempDir(), "my-store")
	if err := m2.Initialize(custom); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom store missing: %v", err)
	}
	cfg2, err := m2.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.StorePath != custom {
		t.Fatalf("custom store path: got %q, want %q", cfg2.StorePath, custom)
	}
}

func TestInitializeRejectsRelativeStore(t *testing.T) {
	wantCode(t, New(cfgPath(t)).Initialize("relative/store"), app.CodeInvalidArgument)
}

func TestRestartParsesConfig(t *testing.T) {
	path := cfgPath(t)
	if err := New(path).Initialize(""); err != nil {
		t.Fatal(err)
	}
	// a fresh Manager (a new process) must detect ready by parsing config
	m := New(path)
	if st, err := m.Detect(); err != nil || st != StateReady {
		t.Fatalf("restart detect: state %q, err %v", st, err)
	}
	a, err := m.App()
	if err != nil {
		t.Fatal(err)
	}
	if a.StateDBPath != filepath.Join(filepath.Dir(path), "state.db") {
		t.Fatalf("restart app state db: got %q", a.StateDBPath)
	}
}

func TestStateMissing(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(path), "state.db")); err != nil {
		t.Fatal(err)
	}
	if st, err := m.Detect(); err != nil || st != StateMissing {
		t.Fatalf("state_missing detect: state %q, err %v", st, err)
	}
	wantCode(t, m.Initialize(""), app.CodeStateMissing)
	_, err := m.App()
	wantCode(t, err, app.CodeStateMissing)
}

func TestInvalidConfig(t *testing.T) {
	path := cfgPath(t)
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("this is [not toml")
	st, err := New(path).Detect()
	if st != StateInvalid || err == nil {
		t.Fatalf("unparseable config: state %q, err %v", st, err)
	}
	wantCode(t, New(path).Initialize(""), app.CodeInvalidConfig)
	_, err = New(path).App()
	wantCode(t, err, app.CodeInvalidConfig)

	write("version = 1\n") // required keys missing
	if st, err := New(path).Detect(); st != StateInvalid || err == nil {
		t.Fatalf("missing keys: state %q, err %v", st, err)
	}

	write("version = 99\nstore_path = \"/x\"\nstate_db_path = \"/y\"\n") // newer config
	if st, err := New(path).Detect(); st != StateInvalid || err == nil {
		t.Fatalf("newer version: state %q, err %v", st, err)
	}
}

func TestAlreadyInitialized(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	wantCode(t, m.Initialize(""), app.CodeAlreadyInitialized)
}

// installConfig writes a valid configuration at path pointing at store and
// stateDB.
func installConfig(t *testing.T, path, store, stateDB string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigAtomic(path, Config{Version: ConfigFormatVersion, StorePath: store, StateDBPath: stateDB}); err != nil {
		t.Fatal(err)
	}
}

// TestTwoManagerInitializeContentionDoesNotRewrite proves the initialization
// race fix: two managers (two processes) racing one installation serialize on
// the cross-process writer lock, and the loser re-reads config/state under
// the lock and refuses instead of rewriting what the winner wrote.
func TestTwoManagerInitializeContentionDoesNotRewrite(t *testing.T) {
	path := cfgPath(t)
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	first := New(path)
	if err := first.Initialize(""); err != nil {
		t.Fatal(err)
	}
	cfgBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "state.db")
	dbBefore, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// A second manager loses the race: it must re-detect the completed
	// initialization under the writer lock and refuse.
	contender := filepath.Join(t.TempDir(), "contender-store")
	second := New(path)
	wantCode(t, second.Initialize(contender), app.CodeAlreadyInitialized)

	cfgAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cfgBefore, cfgAfter) {
		t.Fatalf("config was rewritten by the losing manager:\nbefore: %s\nafter:  %s", cfgBefore, cfgAfter)
	}
	dbAfter, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !dbBefore.ModTime().Equal(dbAfter.ModTime()) || dbBefore.Size() != dbAfter.Size() {
		t.Fatalf("state database was rewritten by the losing manager: before %v/%d, after %v/%d", dbBefore.ModTime(), dbBefore.Size(), dbAfter.ModTime(), dbAfter.Size())
	}
	if _, err := os.Stat(contender); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("losing manager created its Store: %v", err)
	}

	// The installation is still fully usable and points at the winner's Store.
	cfg, err := first.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StorePath != filepath.Join(base, "store") {
		t.Fatalf("config store path: got %q", cfg.StorePath)
	}
	if a, err := first.App(); err != nil || a.StateDBPath != dbPath {
		t.Fatalf("winner app after contention: %v, %v", a, err)
	}
}

func TestDetectNewerStateSchemaInvalid(t *testing.T) {
	path := cfgPath(t)
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "state.db")
	db, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (42);`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	installConfig(t, path, filepath.Join(base, "store"), dbPath)

	m := New(path)
	st, err := m.Detect()
	if st != StateInvalid || err == nil {
		t.Fatalf("newer schema: state %q, err %v", st, err)
	}
	wantCode(t, err, app.CodeInvalidConfig)
	if !strings.Contains(err.Error(), "newer") {
		t.Fatalf("newer-schema error message: %v", err)
	}
	wantCode(t, m.Initialize(""), app.CodeInvalidConfig)
}

func TestDetectCorruptStateSchemaInvalid(t *testing.T) {
	path := cfgPath(t)
	base := filepath.Dir(path)
	dbPath := filepath.Join(base, "state.db")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatal(err)
	}
	installConfig(t, path, filepath.Join(base, "store"), dbPath)

	st, err := New(path).Detect()
	if st != StateInvalid || err == nil {
		t.Fatalf("corrupt state: state %q, err %v", st, err)
	}
	wantCode(t, err, app.CodeInvalidConfig)
}

func TestDetectValidStateReady(t *testing.T) {
	// current schema
	path := cfgPath(t)
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "state.db")
	db, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	installConfig(t, path, filepath.Join(base, "store"), dbPath)
	if st, err := New(path).Detect(); err != nil || st != StateReady {
		t.Fatalf("current schema detect: state %q, err %v", st, err)
	}

	// older: a valid but never-migrated database stays ready, and the real
	// Open still migrates it on first use.
	oldPath := cfgPath(t)
	oldBase := filepath.Dir(oldPath)
	oldDB := filepath.Join(oldBase, "state.db")
	if err := os.MkdirAll(oldBase, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldDB, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	installConfig(t, oldPath, filepath.Join(oldBase, "store"), oldDB)
	oldM := New(oldPath)
	if st, err := oldM.Detect(); err != nil || st != StateReady {
		t.Fatalf("older schema detect: state %q, err %v", st, err)
	}
	a, err := oldM.App()
	if err != nil {
		t.Fatalf("older schema App (Open must migrate): %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if st, err := oldM.Detect(); err != nil || st != StateReady {
		t.Fatalf("older schema detect after migration: state %q, err %v", st, err)
	}
}

func TestInitializeLockedByAnotherProcess(t *testing.T) {
	path := cfgPath(t)
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(filepath.Join(base, "skillctl.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()

	wantCode(t, New(path).Initialize(""), app.CodeLocked)
}

func TestConfigRoundTrip(t *testing.T) {
	cfg := Config{Version: 1, StorePath: "/abs/store", StateDBPath: "/abs/state.db"}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeConfigAtomic(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Fatalf("round trip: got %+v, want %+v", got, cfg)
	}
	// no temp files left behind
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.toml.") {
			t.Fatalf("temp config file left behind: %s", e.Name())
		}
	}
}
