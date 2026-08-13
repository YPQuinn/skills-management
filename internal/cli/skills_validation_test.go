package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/bootstrap"
)

// TestSkillsCLIValidation covers the strict flag contract: required
// --source, exactly one of selectors or --all, single-selector-only
// --slug/--replace, canonical --skill flag name, and stable exit codes.
func TestSkillsCLIValidation(t *testing.T) {
	// uninitialized installation is a blocked use case: exit 1
	bm := bootstrap.New(filepath.Join(t.TempDir(), ".skillctl", "config.toml"))
	out, err := runCmd(t, NewSkillListCmd(bm))
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("uninitialized list: %v\n%s", err, out)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("uninitialized exit code: got %d", ExitCode(err))
	}

	bm = initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local-one"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"missing source", []string{"--path", "skills/alpha"}},
		{"missing selector", []string{"--source", "local-one"}},
		{"all with path", []string{"--source", "local-one", "--all", "--path", "skills/alpha"}},
		{"all with skill", []string{"--source", "local-one", "--all", "--skill", "alpha"}},
		{"all with slug", []string{"--source", "local-one", "--all", "--slug", "x"}},
		{"all with replace", []string{"--source", "local-one", "--all", "--replace"}},
		{"slug with two paths", []string{"--source", "local-one", "--path", "skills/alpha", "--path", "skills/beta", "--slug", "x"}},
		{"replace with two paths", []string{"--source", "local-one", "--path", "skills/alpha", "--path", "skills/beta", "--replace"}},
		{"replace with two skills", []string{"--source", "local-one", "--skill", "alpha", "--skill", "beta", "--replace"}},
		{"explicit empty slug", []string{"--source", "local-one", "--path", "skills/alpha", "--slug", ""}},
		{"explicit empty slug with two paths", []string{"--source", "local-one", "--path", "skills/alpha", "--path", "skills/beta", "--slug", ""}},
		{"all with explicit empty slug", []string{"--source", "local-one", "--all", "--slug", ""}},
	}
	for _, c := range cases {
		out, err := runCmd(t, NewSkillImportCmd(bm), c.args...)
		if err == nil || ExitCode(err) != 2 {
			t.Errorf("%s: got %v (exit %d), want exit 2\n%s", c.name, err, ExitCode(err), out)
		}
	}

	// none of the rejected invocations may have mutated the Store
	out, err = runCmd(t, NewSkillListCmd(bm))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No Skills.") {
		t.Fatalf("rejected imports must not mutate the Store:\n%s", out)
	}

	// the canonical selector flag is --skill; --name must be rejected through
	// the full command tree, where the root maps flag errors to exit 2
	stdout, _, err := runRoot(t, bm, "skill", "import", "--source", "local-one", "--path", "skills/alpha", "--name", "alpha")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("--name must not be a valid flag: %v\n%s", err, stdout)
	}

	// show arity and missing Skill
	for _, c := range []struct {
		name string
		args []string
	}{
		{"show without arg", nil},
		{"show with two args", []string{"alpha", "beta"}},
	} {
		_, err := runCmd(t, NewSkillShowCmd(bm), c.args...)
		if err == nil || ExitCode(err) != 2 {
			t.Errorf("%s: got %v (exit %d), want exit 2", c.name, err, ExitCode(err))
		}
	}
	out, err = runCmd(t, NewSkillShowCmd(bm), "missing")
	if err == nil || ExitCode(err) != 1 || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("show missing: %v\n%s", err, out)
	}
}
