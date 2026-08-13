package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillsCLIImportListShowLifecycle drives the full skill vocabulary
// against one initialized installation: import by path and by unique name,
// list, show by slug and by id, re-import no-op, and --all batch.
func TestSkillsCLIImportListShowLifecycle(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))
	root := t.TempDir()
	writeCLISkill(t, root, "alpha")
	writeCLISkill(t, root, "beta")
	if out, err := runCmd(t, NewSourceAddCmd(bm), root, "--name", "local-one"); err != nil {
		t.Fatalf("source add: %v\n%s", err, out)
	}

	// import one entry by stable relative path
	out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "local-one", "--path", "skills/alpha")
	if err != nil {
		t.Fatalf("import by path: %v\n%s", err, out)
	}
	for _, want := range []string{"Imported skills/alpha", `"alpha"`, "Summary:", "1 imported"} {
		if !strings.Contains(out, want) {
			t.Fatalf("import output missing %q:\n%s", want, out)
		}
	}

	// import one entry by unique Inventory name
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "local-one", "--skill", "beta")
	if err != nil {
		t.Fatalf("import by name: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Imported skills/beta") {
		t.Fatalf("import by name output:\n%s", out)
	}

	// the same entry again is an already-imported no-op
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "local-one", "--path", "skills/alpha")
	if err != nil {
		t.Fatalf("re-import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Already imported") || !strings.Contains(out, "1 already imported") {
		t.Fatalf("re-import output:\n%s", out)
	}

	// list shows both Skills with a summary
	out, err = runCmd(t, NewSkillListCmd(bm))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"alpha", "beta", "2 Skill(s)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q:\n%s", want, out)
		}
	}

	// show by slug and by id
	out, err = runCmd(t, NewSkillShowCmd(bm), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Skill 1: alpha", "Name: alpha", "skills/alpha", "local-one"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}
	out, err = runCmd(t, NewSkillShowCmd(bm), "2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Skill 2: beta") {
		t.Fatalf("show by id output:\n%s", out)
	}

	// JSON list is {items, total} with snake_case fields
	out, err = runCmd(t, NewSkillListCmd(bm), "--json")
	if err != nil {
		t.Fatal(err)
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
			} `json:"binding"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json list: %v\n%s", err, out)
	}
	if envelope.Total != 2 || len(envelope.Items) != 2 {
		t.Fatalf("json list: %+v", envelope)
	}
	if envelope.Items[0].StoreDigest == "" || envelope.Items[0].CreatedAt == "" {
		t.Fatalf("json list item fields: %+v", envelope.Items[0])
	}
	if envelope.Items[0].Binding == nil || envelope.Items[0].Binding.SourceName != "local-one" {
		t.Fatalf("json list binding: %+v", envelope.Items[0].Binding)
	}

	// JSON show agrees with the list item shape
	out, err = runCmd(t, NewSkillShowCmd(bm), "alpha", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Baseline    string `json:"baseline_digest"`
		Binding     struct {
			SourceID    int64  `json:"source_id"`
			RelativeDir string `json:"relative_dir"`
			Digest      string `json:"digest"`
			ImportedAt  string `json:"imported_at"`
		} `json:"binding"`
	}
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("json show: %v\n%s", err, out)
	}
	if shown.Slug != "alpha" || shown.Description == "" || shown.Baseline == "" {
		t.Fatalf("json show: %+v", shown)
	}
	if shown.Binding.RelativeDir != "skills/alpha" || shown.Binding.Digest == "" || shown.Binding.ImportedAt == "" {
		t.Fatalf("json show binding: %+v", shown.Binding)
	}

	// --all imports a whole second Source in one batch
	other := t.TempDir()
	writeCLISkill(t, other, "gamma")
	writeCLISkill(t, other, "delta")
	if out, err := runCmd(t, NewSourceAddCmd(bm), other, "--name", "batch"); err != nil {
		t.Fatalf("source add batch: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "batch", "--all")
	if err != nil {
		t.Fatalf("import --all: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2 imported") || !strings.Contains(out, "Imported skills/delta") {
		t.Fatalf("import --all output:\n%s", out)
	}

	// import --json keeps the stable per-item result shape
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "batch", "--path", "skills/gamma", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items []struct {
			Status      string `json:"status"`
			RelativeDir string `json:"relative_dir"`
			Slug        string `json:"slug"`
			SkillID     int64  `json:"skill_id"`
		} `json:"items"`
		Summary struct {
			Total    int `json:"total"`
			Imported int `json:"imported"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("import --json: %v\n%s", err, out)
	}
	if result.Summary.Total != 1 || result.Summary.Imported != 0 {
		t.Fatalf("import --json summary: %+v", result.Summary)
	}
	if result.Items[0].Status != "already_imported" || result.Items[0].SkillID == 0 {
		t.Fatalf("import --json item: %+v", result.Items[0])
	}
}

// TestSkillsCLISlugOverrideConflictAndReplace proves single-selector
// --slug and --replace: a skipped conflict, an explicit Replace that
// retains identity, and an explicit slug override.
func TestSkillsCLISlugOverrideConflictAndReplace(t *testing.T) {
	bm := initManager(t, filepath.Join(t.TempDir(), "store"))

	rootA := t.TempDir()
	writeCLISkill(t, rootA, "alpha")
	if out, err := runCmd(t, NewSourceAddCmd(bm), rootA, "--name", "source-a"); err != nil {
		t.Fatalf("source add a: %v\n%s", err, out)
	}
	out, err := runCmd(t, NewSkillImportCmd(bm), "--source", "source-a", "--path", "skills/alpha")
	if err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}

	// a second Source claims the same slug with different content:
	// without --replace the item is a skipped conflict
	rootB := t.TempDir()
	writeCLISkill(t, rootB, "alpha")
	if err := os.WriteFile(filepath.Join(rootB, "skills", "alpha", "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: desc\n---\n# Alpha v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runCmd(t, NewSourceAddCmd(bm), rootB, "--name", "source-b"); err != nil {
		t.Fatalf("source add b: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "source-b", "--path", "skills/alpha")
	if err != nil {
		t.Fatalf("conflict import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Skipped conflict") || !strings.Contains(out, "1 skipped conflicts") {
		t.Fatalf("conflict output:\n%s", out)
	}

	// explicit --replace swaps content and Binding while retaining the slug
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "source-b", "--path", "skills/alpha", "--replace")
	if err != nil {
		t.Fatalf("replace import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Replaced Skill 1") {
		t.Fatalf("replace output:\n%s", out)
	}
	out, err = runCmd(t, NewSkillShowCmd(bm), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Source "source-b"`) || !strings.Contains(out, "Path: skills/alpha") {
		t.Fatalf("replaced show output:\n%s", out)
	}

	// an explicit slug override on a single selector imports under that slug
	rootC := t.TempDir()
	writeCLISkill(t, rootC, "gamma")
	if out, err := runCmd(t, NewSourceAddCmd(bm), rootC, "--name", "source-c"); err != nil {
		t.Fatalf("source add c: %v\n%s", err, out)
	}
	out, err = runCmd(t, NewSkillImportCmd(bm), "--source", "source-c", "--path", "skills/gamma", "--slug", "custom")
	if err != nil {
		t.Fatalf("import with --slug: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"custom"`) {
		t.Fatalf("slug override output:\n%s", out)
	}
	out, err = runCmd(t, NewSkillShowCmd(bm), "custom")
	if err != nil {
		t.Fatalf("show custom: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Skill 2: custom") {
		t.Fatalf("show custom output:\n%s", out)
	}
}
