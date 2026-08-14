package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLISkillsLocalLifecycle drives the skill vocabulary through the real
// binary: import by --all and by path, list/show JSON shapes, Store
// materialization, re-import no-op, conflict and replace, and the stable
// flag-validation exit codes.
func TestCLISkillsLocalLifecycle(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "store")
	if out, code := run(t, home, "init", "--store", store); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	root := t.TempDir()
	writeE2ESkill(t, root, "alpha")
	writeE2ESkill(t, root, "beta")
	if out, code := run(t, home, "source", "add", root, "--name", "e2e-local"); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}

	// --all imports every entry into the Store
	out, code := run(t, home, "skill", "import", "--source", "e2e-local", "--all")
	if code != 0 {
		t.Fatalf("import --all: exit %d\n%s", code, out)
	}
	for _, want := range []string{"Imported skills/alpha", "Imported skills/beta", "2 imported"} {
		if !strings.Contains(out, want) {
			t.Fatalf("import --all output missing %q:\n%s", want, out)
		}
	}
	entries, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	var skillDirs []string
	for _, e := range entries {
		if e.Name() != ".skillctl" {
			skillDirs = append(skillDirs, e.Name())
		}
	}
	if len(skillDirs) != 2 {
		t.Fatalf("Store must contain exactly the imported Skills: %v", skillDirs)
	}

	// list --json is one parseable {items, total} value
	out, code = run(t, home, "skill", "list", "--json")
	if code != 0 {
		t.Fatalf("skill list --json: exit %d\n%s", code, out)
	}
	var envelope struct {
		Items []struct {
			ID          int64  `json:"id"`
			Slug        string `json:"slug"`
			Name        string `json:"name"`
			StoreDigest string `json:"store_digest"`
			CreatedAt   string `json:"created_at"`
			Binding     *struct {
				SourceID    int64  `json:"source_id"`
				SourceName  string `json:"source_name"`
				RelativeDir string `json:"relative_dir"`
				Digest      string `json:"digest"`
				ImportedAt  string `json:"imported_at"`
			} `json:"binding"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("list --json is not one JSON value: %v\n%s", err, out)
	}
	if envelope.Total != 2 || len(envelope.Items) != 2 {
		t.Fatalf("list --json content: %+v", envelope)
	}
	if envelope.Items[0].StoreDigest == "" || envelope.Items[0].CreatedAt == "" || !strings.HasSuffix(envelope.Items[0].CreatedAt, "Z") {
		t.Fatalf("list --json item fields: %+v", envelope.Items[0])
	}
	if envelope.Items[0].Binding == nil || envelope.Items[0].Binding.SourceName != "e2e-local" ||
		envelope.Items[0].Binding.Digest == "" || envelope.Items[0].Binding.ImportedAt == "" {
		t.Fatalf("list --json binding: %+v", envelope.Items[0].Binding)
	}

	// show by slug and by numeric id agree through --json
	out, code = run(t, home, "skill", "show", "alpha", "--json")
	if code != 0 {
		t.Fatalf("show --json: exit %d\n%s", code, out)
	}
	var shown struct {
		ID                  int64  `json:"id"`
		Slug                string `json:"slug"`
		Baseline            string `json:"baseline_digest"`
		Description         string `json:"description"`
		HasPreviousSnapshot bool   `json:"has_previous_snapshot"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	if shown.Slug != "alpha" || shown.Baseline == "" || shown.Description == "" {
		t.Fatalf("show --json content: %+v", shown)
	}
	if shown.HasPreviousSnapshot {
		t.Fatalf("a fresh import must expose has_previous_snapshot=false: %+v", shown)
	}
	out, code = run(t, home, "skill", "show", fmt.Sprint(shown.ID))
	if code != 0 || !strings.Contains(out, "Skill "+fmt.Sprint(shown.ID)+": alpha") {
		t.Fatalf("show by id: exit %d\n%s", code, out)
	}

	// a repeated import of the same entry is an already-imported no-op
	out, code = run(t, home, "skill", "import", "--source", "e2e-local", "--path", "skills/alpha")
	if code != 0 || !strings.Contains(out, "Already imported") {
		t.Fatalf("re-import: exit %d\n%s", code, out)
	}

	// a conflicting entry is skipped, then explicitly replaced
	other := t.TempDir()
	writeE2ESkill(t, other, "alpha")
	if err := os.WriteFile(filepath.Join(other, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, home, "source", "add", other, "--name", "e2e-rival"); code != 0 {
		t.Fatalf("source add rival: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "skill", "import", "--source", "e2e-rival", "--path", "skills/alpha")
	if code != 0 || !strings.Contains(out, "Skipped conflict") {
		t.Fatalf("conflict import: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "skill", "import", "--source", "e2e-rival", "--path", "skills/alpha", "--replace")
	if code != 0 || !strings.Contains(out, "Replaced Skill") {
		t.Fatalf("replace import: exit %d\n%s", code, out)
	}
	out, code = run(t, home, "skill", "show", "alpha")
	if code != 0 || !strings.Contains(out, "e2e-rival") {
		t.Fatalf("show after replace: exit %d\n%s", code, out)
	}

	// flag-validation failures exit 2
	for _, args := range [][]string{
		{"skill", "import", "--path", "skills/alpha"},
		{"skill", "import", "--source", "e2e-local"},
		{"skill", "import", "--source", "e2e-local", "--all", "--path", "skills/alpha"},
		{"skill", "import", "--source", "e2e-local", "--path", "skills/alpha", "--path", "skills/beta", "--slug", "x"},
		{"skill", "import", "--source", "e2e-local", "--path", "skills/alpha", "--name", "alpha"},
		{"skill", "import", "--source", "e2e-local", "--path", "missing"},
		{"skill", "show"},
	} {
		out, code := run(t, home, args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2\n%s", args, code, out)
		}
	}
	// a missing Skill is a blocked use case: exit 1
	out, code = run(t, home, "skill", "show", "missing")
	if code != 1 || !strings.Contains(out, "not found") {
		t.Fatalf("show missing: exit %d\n%s", code, out)
	}
}

// TestCLISkillsJSONErrorsSeparateProcess proves the --json failure contract
// for the skill group through the real binary: stdout carries exactly one
// JSON error result and human text stays on stderr.
func TestCLISkillsJSONErrorsSeparateProcess(t *testing.T) {
	home := t.TempDir()
	if out, code := run(t, home, "init"); code != 0 {
		t.Fatalf("init: exit %d\n%s", code, out)
	}

	stdout, stderr, code := runOut(t, home, "skill", "show", "missing", "--json")
	if code != 1 {
		t.Fatalf("show missing --json: exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var res struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not exactly one JSON value: %v\n%s", err, stdout)
	}
	if res.Error.Code != "not_found" || res.Error.Message == "" {
		t.Fatalf("JSON error: %+v", res.Error)
	}
	if res.Error.Details == nil || len(res.Error.Details) != 0 {
		t.Fatalf("details must be an empty object: %v", res.Error.Details)
	}
	if !strings.Contains(stderr, "not found") {
		t.Fatalf("human message must stay on stderr: %q", stderr)
	}

	stdout, _, code = runOut(t, home, "skill", "import", "--json")
	if code != 2 {
		t.Fatalf("import without flags --json: exit %d, want 2\nstdout: %s", code, stdout)
	}
	var invalid struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &invalid); err != nil {
		t.Fatalf("stdout is not one JSON value: %v\n%s", err, stdout)
	}
	if invalid.Error.Code != "invalid_argument" {
		t.Fatalf("JSON code: got %q, want invalid_argument", invalid.Error.Code)
	}
}
