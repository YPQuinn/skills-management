package target

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noEnv simulates an empty environment: no overrides are set.
func noEnv(string) (string, bool) { return "", false }

// testHome returns a temp home whose physical path matches what
// EvalSymlinks reports (macOS /var is a symlink to /private/var).
func testHome(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// TestResolveUserPaths pins every adapter's user-level resolution from
// HOME (decision 04).
func TestResolveUserPaths(t *testing.T) {
	home := testHome(t)
	cases := map[string]string{
		"universal":      filepath.Join(home, ".agents", "skills"),
		"claude-code":    filepath.Join(home, ".claude", "skills"),
		"codex":          filepath.Join(home, ".codex", "skills"),
		"cursor":         filepath.Join(home, ".cursor", "skills"),
		"gemini-cli":     filepath.Join(home, ".gemini", "skills"),
		"opencode":       filepath.Join(home, ".config", "opencode", "skills"),
		"pi":             filepath.Join(home, ".pi", "agent", "skills"),
		"github-copilot": filepath.Join(home, ".copilot", "skills"),
	}
	for key, want := range cases {
		a, _ := ByKey(key)
		rt, err := ResolveUser(a, ResolveOptions{Home: home, LookupEnv: noEnv})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if rt.Path != want || rt.Scope != ScopeUser || rt.Adapter != key {
			t.Fatalf("%s: got %+v, want path %s", key, rt, want)
		}
	}
}

// TestResolveUserOverrides pins the adapter override contract: a non-empty
// absolute override wins, an empty override counts as unset, and a
// non-empty relative override is an error.
func TestResolveUserOverrides(t *testing.T) {
	home := testHome(t)
	override := testHome(t)
	a, _ := ByKey("claude-code")

	env := func(key, value string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			if k == key {
				return value, true
			}
			return "", false
		}
	}

	rt, err := ResolveUser(a, ResolveOptions{Home: home, LookupEnv: env("CLAUDE_CONFIG_DIR", override)})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(override, "skills") {
		t.Fatalf("absolute override: got %s", rt.Path)
	}
	// empty override counts as unset
	rt, err = ResolveUser(a, ResolveOptions{Home: home, LookupEnv: env("CLAUDE_CONFIG_DIR", "")})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(home, ".claude", "skills") {
		t.Fatalf("empty override: got %s", rt.Path)
	}
	// relative override is an error, never silently defaulted
	if _, err := ResolveUser(a, ResolveOptions{Home: home, LookupEnv: env("CLAUDE_CONFIG_DIR", "relative")}); err == nil ||
		!strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("relative override: got %v, want an absolute-path error", err)
	}
	// missing HOME is an error
	if _, err := ResolveUser(a, ResolveOptions{LookupEnv: noEnv}); err == nil {
		t.Fatal("missing HOME must fail")
	}
}

// TestResolveProject pins project resolution: the root must exist and be a
// directory, its symlinks resolve, and the adapter suffix is appended
// without needing to exist.
func TestResolveProject(t *testing.T) {
	root := testHome(t)
	realRoot := testHome(t)
	if err := os.Symlink(realRoot, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	a, _ := ByKey("pi")

	rt, err := ResolveProject(a, filepath.Join(root, "link"), ResolveOptions{Home: t.TempDir(), LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(realRoot, ".pi", "skills") {
		t.Fatalf("project path: got %s, want %s", rt.Path, filepath.Join(realRoot, ".pi", "skills"))
	}
	if rt.Scope != ScopeProject || rt.ProjectRoot != realRoot {
		t.Fatalf("project metadata: %+v", rt)
	}

	// missing root
	if _, err := ResolveProject(a, filepath.Join(root, "nope"), ResolveOptions{}); err == nil ||
		!strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing root: got %v", err)
	}
	// a file is not a project root
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProject(a, file, ResolveOptions{}); err == nil ||
		!strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file root: got %v", err)
	}
	// empty root
	if _, err := ResolveProject(a, "", ResolveOptions{}); err == nil {
		t.Fatal("empty project root must fail")
	}
}

// TestResolveCustom pins the custom container contract (decision 04).
func TestResolveCustom(t *testing.T) {
	home := testHome(t)
	// absolute path
	rt, err := ResolveCustom(filepath.Join(home, "skills"), ResolveOptions{Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(home, "skills") || rt.Adapter != "custom" || rt.Scope != ScopeCustom {
		t.Fatalf("absolute custom: %+v", rt)
	}
	// ~/ expansion, once
	rt, err = ResolveCustom("~/skills", ResolveOptions{Home: home, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(home, "skills") {
		t.Fatalf("~/ custom: %s", rt.Path)
	}
	// ~ alone
	rt, err = ResolveCustom("~", ResolveOptions{Home: home, LookupEnv: noEnv})
	if err != nil || rt.Path != home {
		t.Fatalf("bare ~: %s, %v", rt.Path, err)
	}
	// no hidden adapter suffix for custom targets
	if strings.HasSuffix(rt.Path, "skills") {
		t.Fatalf("custom path must be the container itself: %s", rt.Path)
	}

	// another user's ~name is rejected
	if _, err := ResolveCustom("~alice/skills", ResolveOptions{Home: home, LookupEnv: noEnv}); err == nil {
		t.Fatal("~name must be rejected")
	}
	// environment variables and globs are rejected
	for _, in := range []string{"$HOME/skills", filepath.Join(home, "skill*"), filepath.Join(home, "skill[1]")} {
		if _, err := ResolveCustom(in, ResolveOptions{Home: home, LookupEnv: noEnv}); err == nil {
			t.Fatalf("%q must be rejected", in)
		}
	}
	// relative paths are rejected
	if _, err := ResolveCustom("skills", ResolveOptions{Home: home, LookupEnv: noEnv}); err == nil {
		t.Fatal("relative path must be rejected")
	}
}

// TestResolveSymlinkPrefixAndContainerValidation pins the fixed
// physical-path identity: existing prefixes resolve through symlinks, an
// existing non-directory container is invalid, and the filesystem root is
// never a Target.
func TestResolveSymlinkPrefixAndContainerValidation(t *testing.T) {
	dir := testHome(t)
	real := testHome(t)
	if err := os.Symlink(real, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	rt, err := ResolveCustom(filepath.Join(dir, "link", "skills"), ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Path != filepath.Join(real, "skills") {
		t.Fatalf("symlinked prefix: got %s, want %s", rt.Path, filepath.Join(real, "skills"))
	}

	// an existing non-directory container is invalid
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCustom(file, ResolveOptions{}); err == nil ||
		!strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("non-directory container: got %v", err)
	}

	// the filesystem root is invalid
	if _, err := ResolveCustom("/", ResolveOptions{}); err == nil ||
		!strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("root container: got %v", err)
	}
}

// TestStoreDisjointness pins the Target/Store safety boundary: equality
// and ancestor/descendant relationships are all invalid, both directions.
func TestStoreDisjointness(t *testing.T) {
	base := t.TempDir()
	store := filepath.Join(base, "store")
	cases := []struct {
		name, target string
		wantErr      bool
	}{
		{"equal", store, true},
		{"target inside store", filepath.Join(store, "sub"), true},
		{"store inside target", base, true},
		{"sibling", filepath.Join(base, "target"), false},
		{"unrelated", filepath.Join(t.TempDir(), "target"), false},
	}
	for _, c := range cases {
		rt, err := ResolveCustom(c.target, ResolveOptions{})
		if err != nil {
			t.Fatalf("%s: resolving target: %v", c.name, err)
		}
		err = CheckStoreDisjoint(rt.Path, store, ResolveOptions{})
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: got %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

// TestCompatibleAdapters pins the derived, non-persisted adapter
// compatibility of a shared path (decision 04): a project .agents/skills
// Target is compatible with Universal and every adapter sharing that
// project suffix.
func TestCompatibleAdapters(t *testing.T) {
	project := testHome(t)
	shared := filepath.Join(project, ".agents", "skills")
	got := CompatibleAdapters(shared, ScopeProject, project, ResolveOptions{Home: project, LookupEnv: noEnv})
	want := []string{"universal", "codex", "cursor", "gemini-cli", "opencode", "github-copilot"}
	if len(got) != len(want) {
		t.Fatalf("compatible adapters: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("compatible adapters: got %v, want %v", got, want)
		}
	}
	// a project .pi/skills path matches only Pi
	piPath := filepath.Join(project, ".pi", "skills")
	got = CompatibleAdapters(piPath, ScopeProject, project, ResolveOptions{Home: project, LookupEnv: noEnv})
	if len(got) != 1 || got[0] != "pi" {
		t.Fatalf("pi project compat: %v", got)
	}
	// a user ~/.pi/agent/skills path matches only Pi
	home := testHome(t)
	userPi := filepath.Join(home, ".pi", "agent", "skills")
	got = CompatibleAdapters(userPi, ScopeUser, "", ResolveOptions{Home: home, LookupEnv: noEnv})
	if len(got) != 1 || got[0] != "pi" {
		t.Fatalf("pi user compat: %v", got)
	}
	// custom scope derives no adapters
	if got := CompatibleAdapters(shared, ScopeCustom, "", ResolveOptions{Home: home, LookupEnv: noEnv}); len(got) != 0 {
		t.Fatalf("custom compat: %v", got)
	}
}
