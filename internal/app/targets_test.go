package app

import (
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/state"
)

// setTestHome points HOME at a temp directory whose physical path matches
// EvalSymlinks (macOS /var is a symlink to /private/var).
func setTestHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home
}

func TestRegisterCustomTarget(t *testing.T) {
	a := newTestApp(t)
	home := setTestHome(t)
	container := filepath.Join(home, "skills")

	view, err := a.RegisterTarget(TargetInput{Path: "~/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Path != container || view.Adapter != "custom" || view.Scope != "custom" {
		t.Fatalf("custom target: %+v", view)
	}
	if view.Name != "custom-skills" {
		t.Fatalf("default name: %q", view.Name)
	}
	if len(view.CompatibleAdapters) != 0 {
		t.Fatalf("custom compatible adapters: %v", view.CompatibleAdapters)
	}
	if len(view.DirectSkills) != 0 || len(view.Groups) != 0 || len(view.DesiredSkills) != 0 {
		t.Fatalf("fresh custom target must be empty: %+v", view)
	}

	// the fixed physical path identity deduplicates: the same resolved
	// path returns the existing Target regardless of spelling or name
	again, err := a.RegisterTarget(TargetInput{Path: container, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != view.ID {
		t.Fatalf("same path must return the existing Target: %d != %d", again.ID, view.ID)
	}
	// an existing non-directory path is invalid
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RegisterTarget(TargetInput{Path: file}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("file path: got %v", err)
	}
	// the filesystem root is invalid
	if _, err := a.RegisterTarget(TargetInput{Path: "/"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("root path: got %v", err)
	}
	// unsafe path syntax is rejected
	for _, p := range []string{"$HOME/skills", "~/skill*", "~alice/skills", "relative"} {
		if _, err := a.RegisterTarget(TargetInput{Path: p}); err == nil || err.(*Error).Code != CodeInvalidArgument {
			t.Fatalf("unsafe path %q: got %v", p, err)
		}
	}
}

func TestRegisterBuiltinTargets(t *testing.T) {
	a := newTestApp(t)
	home := setTestHome(t)

	view, err := a.RegisterTarget(TargetInput{Adapter: "pi", Scope: "user"})
	if err != nil {
		t.Fatal(err)
	}
	wantUser := filepath.Join(home, ".pi", "agent", "skills")
	if view.Path != wantUser || view.Scope != "user" || view.Adapter != "pi" {
		t.Fatalf("pi user target: %+v, want %s", view, wantUser)
	}
	if view.Name != "pi-user" {
		t.Fatalf("default user name: %q", view.Name)
	}
	compat := view.CompatibleAdapters
	if len(compat) != 1 || compat[0] != "pi" {
		t.Fatalf("compatible adapters: %v", compat)
	}

	project := setTestHome(t)
	pv, err := a.RegisterTarget(TargetInput{Adapter: "codex", Scope: "project", ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if pv.Path != filepath.Join(project, ".agents", "skills") || pv.Scope != "project" || pv.ProjectRoot != project {
		t.Fatalf("codex project target: %+v", pv)
	}
	if pv.Name != "codex-"+filepath.Base(project) {
		t.Fatalf("default project name: %q", pv.Name)
	}
	// the shared .agents/skills project path is compatible with every
	// adapter sharing that suffix
	wantCompat := []string{"universal", "codex", "cursor", "gemini-cli", "opencode", "github-copilot"}
	if len(pv.CompatibleAdapters) != len(wantCompat) {
		t.Fatalf("project compatible adapters: %v, want %v", pv.CompatibleAdapters, wantCompat)
	}
	for i := range wantCompat {
		if pv.CompatibleAdapters[i] != wantCompat[i] {
			t.Fatalf("project compatible adapters: %v, want %v", pv.CompatibleAdapters, wantCompat)
		}
	}

	// registration validation
	if _, err := a.RegisterTarget(TargetInput{}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("empty input: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Adapter: "nope", Scope: "user"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("unknown adapter: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Adapter: "pi"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("missing scope: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Adapter: "pi", Scope: "project"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("missing project root: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Path: "~/x", Adapter: "pi", Scope: "user"}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("mixed forms: got %v", err)
	}
	// a missing project root is invalid
	if _, err := a.RegisterTarget(TargetInput{Adapter: "pi", Scope: "project", ProjectRoot: filepath.Join(home, "nope")}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("missing project root dir: got %v", err)
	}
	// explicit names conflict
	if _, err := a.RegisterTarget(TargetInput{Adapter: "claude-code", Scope: "user", Name: "pi-user"}); err == nil || err.(*Error).Code != CodeConflict {
		t.Fatalf("name conflict: got %v", err)
	}
}

// TestRegisterTargetStoreDisjoint pins the safety boundary: a Target that
// is equal to, inside, or containing the Skill Store is rejected.
func TestRegisterTargetStoreDisjoint(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.RegisterTarget(TargetInput{Path: a.StorePath}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("store itself: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Path: filepath.Join(a.StorePath, "sub")}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("inside store: got %v", err)
	}
	if _, err := a.RegisterTarget(TargetInput{Path: filepath.Dir(a.StorePath)}); err == nil || err.(*Error).Code != CodeInvalidArgument {
		t.Fatalf("containing store: got %v", err)
	}
	// a sibling is fine
	if _, err := a.RegisterTarget(TargetInput{Path: filepath.Join(filepath.Dir(a.StorePath), "target")}); err != nil {
		t.Fatalf("sibling: %v", err)
	}
}

func TestListShowAndResolveTargets(t *testing.T) {
	a := newTestApp(t)
	home := setTestHome(t)
	if _, err := a.RegisterTarget(TargetInput{Path: "~/one", Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RegisterTarget(TargetInput{Path: "~/two", Name: "two"}); err != nil {
		t.Fatal(err)
	}
	items, err := a.ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "one" || items[1].Name != "two" {
		t.Fatalf("list: %+v", items)
	}
	id, err := a.ResolveTargetArg("one")
	if err != nil || id != items[0].ID {
		t.Fatalf("resolve by name: %d, %v", id, err)
	}
	if _, err := a.ResolveTargetArg("nope"); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("resolve missing: %v", err)
	}
	shown, err := a.ShowTarget(id)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Path != filepath.Join(home, "one") {
		t.Fatalf("show: %+v", shown)
	}
	if _, err := a.ShowTarget(999); err == nil || err.(*Error).Code != CodeNotFound {
		t.Fatalf("show missing: %v", err)
	}
}

// TestTargetPathIdentityFixed pins that identity is the resolved physical
// path and never moves with a later adapter-table or environment change:
// a symlinked prefix resolves at registration time.
func TestTargetPathIdentityFixed(t *testing.T) {
	a := newTestApp(t)
	home := setTestHome(t)
	real := setTestHome(t)
	link := filepath.Join(home, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	view, err := a.RegisterTarget(TargetInput{Path: filepath.Join(link, "skills")})
	if err != nil {
		t.Fatal(err)
	}
	if view.Path != filepath.Join(real, "skills") {
		t.Fatalf("physical identity: got %s, want %s", view.Path, filepath.Join(real, "skills"))
	}
	stored, err := state.GetTargetByPath(a.db, filepath.Join(real, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != view.ID {
		t.Fatalf("stored target: %+v", stored)
	}
}
