package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The CLI wire-contract tests pin the --json import result shape against
// the frozen REST import response with literals independent of any view
// struct, so tag/omitempty drift cannot hide behind the same type in tests.

// rawCLIJSON decodes one --json stdout value into an untyped object.
func rawCLIJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, out)
	}
	return m
}

func requireWireKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("wire shape missing key %q in %v", k, m)
		}
	}
}

func forbidWireKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Fatalf("wire shape must omit key %q in %v", k, m)
		}
	}
}

// writeCLISkillName writes one Inventory entry whose frontmatter name is
// the given value rather than the directory name.
func writeCLISkillName(t *testing.T, root, relDir, name string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: \"" + name + "\"\ndescription: desc\n---\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSkillImportJSONWireContract pins the CLI --json import result: a
// success item carries status/relative_dir/requested_slug/slug/skill_id and
// omits code/message/replaces; a conflict item adds compact replaces with
// exactly skill_id/slug/name; a failed item carries code/message and omits
// slug identity; the summary always carries all six tally keys.
func TestSkillImportJSONWireContract(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "one"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}

	// success item and full summary key set
	out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "one", "--path", "skills/alpha", "--json")
	if err != nil {
		t.Fatalf("import --json: %v\n%s", err, out)
	}
	env := rawCLIJSON(t, out)
	item := env["items"].([]any)[0].(map[string]any)
	requireWireKeys(t, item, "status", "relative_dir", "requested_slug", "slug", "skill_id")
	forbidWireKeys(t, item, "code", "message", "replaces", "error_code", "error_message")
	if item["status"] != "imported" || item["relative_dir"] != "skills/alpha" {
		t.Fatalf("success item: %v", item)
	}
	summary := env["summary"].(map[string]any)
	requireWireKeys(t, summary, "total", "imported", "already_imported", "skipped_conflict", "replaced", "failed")
	if summary["imported"].(float64) != 1 {
		t.Fatalf("summary: %v", summary)
	}

	// a rival Source claims the same slug with different content
	rival := t.TempDir()
	writeCLISkill(t, rival, "alpha")
	if err := os.WriteFile(filepath.Join(rival, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runCmd(t, NewSourceAddCmd(bm), rival, "--name", "rival"); err != nil {
		t.Fatalf("source add rival: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "rival", "--path", "skills/alpha", "--json")
	if err != nil {
		t.Fatalf("conflict import --json: %v\n%s", err, out)
	}
	citem := rawCLIJSON(t, out)["items"].([]any)[0].(map[string]any)
	requireWireKeys(t, citem, "status", "relative_dir", "requested_slug", "slug", "skill_id", "replaces")
	forbidWireKeys(t, citem, "code", "message")
	if citem["status"] != "skipped_conflict" {
		t.Fatalf("conflict item: %v", citem)
	}
	replaces := citem["replaces"].(map[string]any)
	requireWireKeys(t, replaces, "skill_id", "slug", "name")
	forbidWireKeys(t, replaces, "id", "description", "store_digest", "binding")
	if replaces["slug"] != "alpha" {
		t.Fatalf("replaces: %v", replaces)
	}

	// a name that derives no default slug fails per-item
	weird := t.TempDir()
	writeCLISkillName(t, weird, "skills/weird", "!!!")
	if out, err := runCmd(t, NewSourceAddCmd(bm), weird, "--name", "weird-src"); err != nil {
		t.Fatalf("source add weird: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "weird-src", "--path", "skills/weird", "--json")
	if err != nil {
		t.Fatalf("failed import --json: %v\n%s", err, out)
	}
	fitem := rawCLIJSON(t, out)["items"].([]any)[0].(map[string]any)
	requireWireKeys(t, fitem, "status", "relative_dir", "code", "message")
	forbidWireKeys(t, fitem, "requested_slug", "slug", "skill_id", "replaces")
	if fitem["status"] != "failed" || fitem["code"] != "invalid_argument" {
		t.Fatalf("failed item: %v", fitem)
	}
	if _, ok := fitem["message"].(string); !ok || fitem["message"] == "" {
		t.Fatalf("failed message: %v", fitem["message"])
	}
	fsummary := rawCLIJSON(t, out)["summary"].(map[string]any)
	requireWireKeys(t, fsummary, "total", "imported", "already_imported", "skipped_conflict", "replaced", "failed")
	if fsummary["failed"].(float64) != 1 {
		t.Fatalf("failed summary: %v", fsummary)
	}
}
