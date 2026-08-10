package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

func TestVersionFlag(t *testing.T) {
	cmd := newRootCmd(bootstrap.New(filepath.Join(t.TempDir(), "config.toml")))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--version: %v", err)
	}
	want := fmt.Sprintf("skillctl %s (%s)\n", version, gitCommit)
	if out.String() != want {
		t.Fatalf("--version output: got %q, want %q", out.String(), want)
	}
}

func TestStatusInvalidConfigFails(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), ".skillctl", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("this is [not toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewStatusCmd(bootstrap.New(cfgPath))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	var ae *app.Error
	if !errors.As(err, &ae) || ae.Code != app.CodeInvalidConfig {
		t.Fatalf("status on invalid config: got %v, want invalid_config", err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code: got %d, want 2", ExitCode(err))
	}
	if out.String() != "" {
		t.Fatalf("status printed output on invalid config: %q", out.String())
	}
}

func TestStatusPrintsReadableStates(t *testing.T) {
	cmd := NewStatusCmd(bootstrap.New(filepath.Join(t.TempDir(), ".skillctl", "config.toml")))
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Status: uninitialized\n" {
		t.Fatalf("status output: got %q", out.String())
	}
}
