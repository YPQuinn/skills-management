package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
)

const cliSkill = "---\nname: %s\ndescription: %s\n---\n# Body\n"

func initManager(t *testing.T, storePath string) *bootstrap.Manager {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), ".skillctl", "config.toml")
	bm := bootstrap.New(cfgPath)
	if err := bm.Initialize(storePath); err != nil {
		t.Fatal(err)
	}
	return bm
}

func writeCLISkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte(strings.ReplaceAll(strings.ReplaceAll(cliSkill, "%s", name), "%s", "desc")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestSourcesCLILifecycle(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	writeCLISkill(t, root, "beta")

	// add
	out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local-one")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Registered Source \"local-one\"") || !strings.Contains(out, "2 Skill(s)") {
		t.Fatalf("add output: %q", out)
	}

	// list
	out, err = runCmd(t, NewSourceListCmd(bm))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "local-one") || !strings.Contains(out, "2 Skill(s)") {
		t.Fatalf("list output: %q", out)
	}

	// show by name
	out, err = runCmd(t, NewSourceShowCmd(bm), "local-one")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Source 1: local-one", "skills/alpha", "skills/beta", "Check result: ok", "Inventory digest: "} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	// check by name
	out, err = runCmd(t, NewSourceCheckCmd(bm), "local-one")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "available with 2 Skill(s)") {
		t.Fatalf("check output: %q", out)
	}

	// json output is exactly one parseable JSON value
	out, err = runCmd(t, NewSourceListCmd(bm), "--json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json list: %v\n%s", err, out)
	}
	if envelope.Total != 1 || envelope.Items[0].Name != "local-one" {
		t.Fatalf("json list: %+v", envelope)
	}

	out, err = runCmd(t, NewSourceShowCmd(bm), "local-one", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Name      string `json:"name"`
		Inventory []struct {
			RelativeDir string `json:"relative_dir"`
			Digest      string `json:"digest"`
		} `json:"inventory"`
		LastCheckResult     string `json:"last_check_result"`
		LastInventoryDigest string `json:"last_inventory_digest"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("json show: %v\n%s", err, out)
	}
	if shown.Name != "local-one" || len(shown.Inventory) != 2 {
		t.Fatalf("json show: %+v", shown)
	}
	if shown.LastCheckResult != "ok" || shown.LastInventoryDigest == "" {
		t.Fatalf("json show metadata: %+v", shown)
	}
	for _, e := range shown.Inventory {
		if e.Digest == "" {
			t.Fatalf("json show entry digest missing: %+v", shown.Inventory)
		}
	}

	// an unavailable Source is reported but the check succeeds
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, NewSourceCheckCmd(bm), "1")
	if err != nil {
		t.Fatalf("check unavailable must still succeed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "unavailable") {
		t.Fatalf("unavailable check output: %q", out)
	}
	out, err = runCmd(t, NewSourceShowCmd(bm), "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "stale") || !strings.Contains(out, "skills/alpha") {
		t.Fatalf("stale show output: %q", out)
	}
}

func TestSourcesCLIErrors(t *testing.T) {
	// uninitialized installation
	bm := bootstrap.New(filepath.Join(t.TempDir(), ".skillctl", "config.toml"))
	out, err := runCmd(t, NewSourceListCmd(bm))
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("uninitialized list: %v\n%s", err, out)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("uninitialized exit code: got %d", ExitCode(err))
	}

	bm = initManager(t, filepath.Join(t.TempDir(), "store"))
	// argument validation exits 2
	out, err = runCmd(t, NewSourceAddCmd(bm))
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("add without location: %v", err)
	}
	out, err = runCmd(t, NewSourceAddCmd(bm), "/x", "--kind", "svn")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("bad kind: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSourceAddCmd(bm), t.TempDir(), "--subpath", "skills")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "cannot have a subpath") {
		t.Fatalf("local with subpath: %v\n%s", err, out)
	}
	// show and check use the typed arity validator and exit 2 on wrong arity
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"show without id", NewSourceShowCmd(bm), nil},
		{"show with two ids", NewSourceShowCmd(bm), []string{"1", "2"}},
		{"check without id", NewSourceCheckCmd(bm), nil},
		{"check with two ids", NewSourceCheckCmd(bm), []string{"1", "2"}},
	} {
		out, err = runCmd(t, tc.cmd, tc.args...)
		if err == nil || ExitCode(err) != 2 {
			t.Errorf("%s: got %v, want exit 2", tc.name, err)
		}
	}
	out, err = runCmd(t, NewSourceShowCmd(bm), "missing")
	if err == nil || ExitCode(err) != 1 {
		t.Fatalf("show missing: %v\n%s", err, out)
	}
}

func TestSourcesJSONEmptyCollections(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))

	// an empty list encodes as [] rather than null
	out, err := runCmd(t, NewSourceListCmd(bm), "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if envelope.Items == nil {
		t.Fatalf("empty items must encode as [], got null:\n%s", out)
	}

	// an empty directory registers as a valid Source with no Skills
	out, err = runCmd(t, NewSourceAddCmd(bm), t.TempDir(), "--name", "empty")
	if err != nil {
		t.Fatalf("add empty: %v\n%s", err, out)
	}

	out, err = runCmd(t, NewSourceShowCmd(bm), "empty", "--json")
	if err != nil {
		t.Fatalf("show empty --json: %v\n%s", err, out)
	}
	var shown struct {
		Inventory []json.RawMessage `json:"inventory"`
		Issues    []json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("show empty --json: %v\n%s", err, out)
	}
	if shown.Inventory == nil {
		t.Fatalf("empty inventory must encode as [], got null:\n%s", out)
	}
	if shown.Issues == nil {
		t.Fatalf("empty issues must encode as [], got null:\n%s", out)
	}
}
