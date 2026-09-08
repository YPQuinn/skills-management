package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeLocal(t *testing.T) {
	dir := t.TempDir()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := Normalize(KindLocal, dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Kind != KindLocal {
		t.Fatalf("kind: got %q", loc.Kind)
	}
	if loc.Location != realDir {
		t.Fatalf("location: got %q, want %q", loc.Location, realDir)
	}

	// relative paths resolve against the working directory
	rel, err := Normalize("", ".", "", "")
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if rel.Kind != KindLocal || !filepath.IsAbs(rel.Location) || rel.Location != wd {
		t.Fatalf("relative location: got %+v, want absolute %q", rel, wd)
	}

	// tilde expands to the home directory
	home, _ := os.UserHomeDir()
	tild, err := Normalize(KindLocal, "~", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if tild.Location != home {
		t.Fatalf("tilde location: got %q, want %q", tild.Location, home)
	}

	// Local Sources cannot have a ref
	if _, err := Normalize(KindLocal, dir, "main", ""); err == nil {
		t.Fatal("Local Source with a ref: want error")
	}
	// Local Sources cannot have a subpath; point location at the scan root
	if _, err := Normalize(KindLocal, dir, "", "skills"); err == nil {
		t.Fatal("Local Source with a subpath: want error")
	}
	loc, err = Normalize(KindLocal, dir, "", "  ")
	if err != nil || loc.Subpath != "" {
		t.Fatalf("whitespace-only subpath: got %+v, %v", loc, err)
	}
}

func TestNormalizeLocalSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := Normalize(KindLocal, link, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Location != want {
		t.Fatalf("symlinked location: got %q, want the real path %q", loc.Location, want)
	}
}

func TestNormalizeGit(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"owner/repo shorthand", "owner/repo", "https://github.com/owner/repo.git"},
		{"github: shorthand", "github:owner/repo", "https://github.com/owner/repo.git"},
		{"github: shorthand with .git", "github:owner/repo.git", "https://github.com/owner/repo.git"},
		{"github: shorthand trailing slash", "github:owner/repo/", "https://github.com/owner/repo.git"},
		{"github https without .git", "https://github.com/owner/repo", "https://github.com/owner/repo.git"},
		{"github https with .git", "https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"ssh url", "ssh://git@example.com/owner/repo.git", "ssh://git@example.com/owner/repo.git"},
		{"scp-like", "git@example.com:owner/repo.git", "git@example.com:owner/repo.git"},
		{"gitlab https", "https://gitlab.com/owner/repo", "https://gitlab.com/owner/repo"},
		{"trailing slash", "https://example.com/owner/repo.git/", "https://example.com/owner/repo.git"},
	}
	for _, c := range cases {
		loc, err := Normalize(KindGit, c.in, "", "")
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if loc.Kind != KindGit || loc.Location != c.want {
			t.Errorf("%s: got %q (%s), want %q (git)", c.name, loc.Location, loc.Kind, c.want)
		}
	}

	// a plain path is not a Git location
	if _, err := Normalize(KindGit, "/not/a/repo", "", ""); err == nil {
		t.Fatal("plain path as Git location: want error")
	}
	// an invalid github: shorthand is refused without echoing a secret
	if _, err := Normalize(KindGit, "github:not-a-shorthand", "", ""); err == nil {
		t.Fatal("invalid github: shorthand: want error")
	}
}

// TestNormalizeRejectsCredentialUserinfo guards fix 2: any Git URL that
// embeds credentials in its userinfo is rejected before persistence, while
// SSH identities (username-only ssh:// and scp-like forms) remain valid.
func TestNormalizeRejectsCredentialUserinfo(t *testing.T) {
	for _, loc := range []string{
		"https://user:pass@github.com/o/r.git",
		"https://ghp_1234567890abcdef@github.com/o/r.git",
		"http://user:pass@example.com/o/r.git",
		"ssh://user:pass@example.com/o/r.git",
		"git://user@example.com/o/r.git",
	} {
		if _, err := Normalize(KindGit, loc, "", ""); err == nil {
			t.Errorf("%s: want error", loc)
		}
	}
	// username-only SSH identities are not stored credentials
	for _, loc := range []string{
		"ssh://git@github.com/o/r.git",
		"git@github.com:o/r.git",
	} {
		if _, err := Normalize(KindGit, loc, "", ""); err != nil {
			t.Errorf("%s: %v", loc, err)
		}
	}

	// the error never echoes the location or the embedded secret
	_, err := Normalize(KindGit, "https://user:supersecret@github.com/o/r.git", "", "")
	if err == nil || strings.Contains(err.Error(), "supersecret") || strings.Contains(err.Error(), "github.com/o/r") {
		t.Fatalf("error must not echo credentials: %v", err)
	}
}

func TestNormalizeKindInference(t *testing.T) {
	dir := t.TempDir()
	// an existing directory wins over owner/repo shorthand
	existing := filepath.Join(t.TempDir(), "owner", "repo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if loc, err := Normalize("", existing, "", ""); err != nil || loc.Kind != KindLocal {
		t.Fatalf("existing directory: got %+v, %v", loc, err)
	}
	if loc, err := Normalize("", "owner/repo", "", ""); err != nil || loc.Kind != KindGit {
		t.Fatalf("owner/repo shorthand: got %+v, %v", loc, err)
	}
	if loc, err := Normalize("", "github:owner/repo", "", ""); err != nil || loc.Kind != KindGit || loc.Location != "https://github.com/owner/repo.git" {
		t.Fatalf("github: shorthand inference: got %+v, %v", loc, err)
	}
	if loc, err := Normalize("", "https://example.com/x.git", "", ""); err != nil || loc.Kind != KindGit {
		t.Fatalf("https URL: got %+v, %v", loc, err)
	}
	if loc, err := Normalize("", dir, "", ""); err != nil || loc.Kind != KindLocal {
		t.Fatalf("existing temp dir: got %+v, %v", loc, err)
	}
	if loc, err := Normalize("", "/no/such/dir/anywhere", "", ""); err != nil || loc.Kind != KindLocal {
		t.Fatalf("missing path: got %+v, %v", loc, err)
	}
}

func TestNormalizeSubpath(t *testing.T) {
	const gitLoc = "https://github.com/owner/repo.git"
	loc, err := Normalize(KindGit, gitLoc, "", "skills/alpha")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Subpath != "skills/alpha" {
		t.Fatalf("subpath: got %q", loc.Subpath)
	}
	loc, err = Normalize(KindGit, gitLoc, "", "/skills//alpha/")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Subpath != "skills/alpha" {
		t.Fatalf("cleaned subpath: got %q", loc.Subpath)
	}
	if _, err := Normalize(KindGit, gitLoc, "", "../escape"); err == nil {
		t.Fatal("traversing subpath: want error")
	}
	if _, err := Normalize(KindGit, gitLoc, "", "a/../../b"); err == nil {
		t.Fatal("escaped subpath: want error")
	}
	loc, err = Normalize(KindGit, gitLoc, "", `skills\alpha`)
	if err != nil {
		t.Fatal(err)
	}
	if loc.Subpath != "skills/alpha" {
		t.Fatalf("backslash subpath: got %q", loc.Subpath)
	}
	// root-equivalent subpaths canonicalize to the empty subpath so the
	// unique tuple cannot be bypassed with equivalent spellings
	for _, sp := range []string{".", "./", "skills/."} {
		loc, err = Normalize(KindGit, gitLoc, "", sp)
		if err != nil {
			t.Fatalf("subpath %q: %v", sp, err)
		}
		want := ""
		if sp == "skills/." {
			want = "skills"
		}
		if loc.Subpath != want {
			t.Fatalf("subpath %q: got %q, want %q", sp, loc.Subpath, want)
		}
	}
}

func TestResolveLocalSubpath(t *testing.T) {
	// the root is the fully resolved path a registered Source stores
	base := t.TempDir()
	root, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveLocalSubpath(root, "")
	if err != nil || got != root {
		t.Fatalf("empty subpath: got %q, %v", got, err)
	}
	got, err = ResolveLocalSubpath(root, "skills")
	wantReal, err := filepath.EvalSymlinks(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || got != wantReal {
		t.Fatalf("plain subpath: got %q, %v", got, err)
	}

	// a symlinked subpath escaping the Source is rejected
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLocalSubpath(root, "escape"); err == nil {
		t.Fatal("symlink escape: want error")
	}

	// a symlink that stays inside the Source resolves to the real path
	if err := os.Symlink(filepath.Join(root, "skills"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveLocalSubpath(root, "link")
	wantReal, err = filepath.EvalSymlinks(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || got != wantReal {
		t.Fatalf("internal symlink: got %q, %v", got, err)
	}

	// a missing subpath is an error (observation reports unavailability)
	if _, err := ResolveLocalSubpath(root, "missing"); err == nil {
		t.Fatal("missing subpath: want error")
	}

	// a root replaced by a symlink is rejected: the physical identity of
	// the registered Source changed
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLocalSubpath(root, ""); err == nil {
		t.Fatal("symlink-replaced root: want error")
	}
}

func TestNormalizeRejectsEmpty(t *testing.T) {
	if _, err := Normalize(KindLocal, "  ", "", ""); err == nil {
		t.Fatal("empty location: want error")
	}
	if _, err := Normalize("bogus", "/x", "", ""); err == nil {
		t.Fatal("unsupported kind: want error")
	}
}
