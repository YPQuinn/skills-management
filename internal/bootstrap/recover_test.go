package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/app"
	"skillctl/internal/lock"
)

func writeRecoverSkill(t *testing.T, store, slug, name string) {
	t.Helper()
	dir := filepath.Join(store, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: desc\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverStoreRebuildsUnboundSkills(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	cfg, err := m.Config()
	if err != nil {
		t.Fatal(err)
	}
	writeRecoverSkill(t, cfg.StorePath, "alpha", "Alpha")
	if err := os.MkdirAll(filepath.Join(cfg.StorePath, ".skillctl", "baselines", "9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StorePath, ".skillctl", "baselines", "9", "old.md"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.StateDBPath); err != nil {
		t.Fatal(err)
	}
	if _, err := m.App(); err == nil {
		t.Fatal("App must refuse state_missing")
	}

	fresh := New(path)
	wantCode(t, fresh.Initialize(""), app.CodeStateMissing)
	result, err := fresh.RecoverStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Recovered) != 1 || result.Recovered[0].Slug != "alpha" || result.Recovered[0].ID <= 9 {
		t.Fatalf("recover result: %+v", result)
	}
	cfgAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cfgBefore, cfgAfter) {
		t.Fatalf("recover rewrote config")
	}
	if st, err := fresh.Detect(); err != nil || st != StateReady {
		t.Fatalf("after recover: %q %v", st, err)
	}
	a, err := fresh.App()
	if err != nil {
		t.Fatal(err)
	}
	skills, err := a.ListSkills()
	if err != nil || len(skills) != 1 || skills[0].Binding != nil || skills[0].SyncStatus != "unbound" {
		t.Fatalf("skills: %+v %v", skills, err)
	}
	if data, err := os.ReadFile(filepath.Join(cfg.StorePath, ".skillctl", "baselines", "9", "old.md")); err != nil || string(data) != "keep\n" {
		t.Fatalf("leftover baseline rewritten: %q %v", data, err)
	}
}

func TestRecoverStoreRefusesOtherStates(t *testing.T) {
	wantCode(t, recoverErr(t, New(cfgPath(t))), app.CodeNotInitialized)

	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	wantCode(t, recoverErr(t, m), app.CodeAlreadyInitialized)

	if err := os.WriteFile(path, []byte("this is [not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantCode(t, recoverErr(t, New(path)), app.CodeInvalidConfig)
}

func TestRecoverStoreIsolatesStaleCachedApp(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	stale, err := m.App()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := m.Config()
	if err != nil {
		t.Fatal(err)
	}
	writeRecoverSkill(t, cfg.StorePath, "kept", "Kept")
	if err := os.Remove(cfg.StateDBPath); err != nil {
		t.Fatal(err)
	}
	result, err := m.RecoverStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Recovered) != 1 || result.Recovered[0].Slug != "kept" {
		t.Fatalf("recover: %+v", result)
	}
	if _, err := stale.ListSkills(); err == nil {
		t.Fatal("stale App handle must not keep operating after recover closed it")
	}
	fresh, err := m.App()
	if err != nil {
		t.Fatal(err)
	}
	if fresh == stale {
		t.Fatal("recover must not return the closed App")
	}
	skills, err := fresh.ListSkills()
	if err != nil || len(skills) != 1 || skills[0].Slug != "kept" {
		t.Fatalf("fresh App after recover: %+v %v", skills, err)
	}
}

func recoverErr(t *testing.T, m *Manager) error {
	t.Helper()
	_, err := m.RecoverStore()
	return err
}

func TestRecoverStoreLockedByAnotherProcess(t *testing.T) {
	path := cfgPath(t)
	m := New(path)
	if err := m.Initialize(""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(path), "state.db")); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryExclusive(filepath.Join(filepath.Dir(path), "skillctl.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()
	wantCode(t, recoverErr(t, New(path)), app.CodeLocked)
}
