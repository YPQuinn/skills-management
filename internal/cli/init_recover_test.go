package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

func TestInitRecoverStoreJSON(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), ".skillctl", "config.toml")
	bm := bootstrap.New(cfgPath)
	if err := bm.Initialize(""); err != nil {
		t.Fatal(err)
	}
	cfg, err := bm.Config()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.StorePath, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: Alpha\ndescription: desc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.StateDBPath); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runRoot(t, bootstrap.New(cfgPath), "init", "--recover-store", "--json")
	if err != nil {
		t.Fatalf("recover: %v\n%s%s", err, stdout, stderr)
	}
	var got app.StoreRecoveryResult
	if jerr := json.Unmarshal([]byte(stdout), &got); jerr != nil {
		t.Fatalf("json: %v\n%s", jerr, stdout)
	}
	if len(got.Recovered) != 1 || got.Recovered[0].Slug != "alpha" {
		t.Fatalf("recovered: %+v", got)
	}
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
}

func TestInitRecoverStoreRejectsStoreFlag(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), ".skillctl", "config.toml")
	stdout, _, err := runRoot(t, bootstrap.New(cfgPath), "init", "--recover-store", "--store", "/tmp/store")
	if err == nil {
		t.Fatal("want error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit %d, want 2", ExitCode(err))
	}
	if !strings.Contains(err.Error(), "--store") {
		t.Fatalf("error: %v", err)
	}
	if stdout != "" {
		t.Fatalf("human failure must not write stdout: %q", stdout)
	}
}
