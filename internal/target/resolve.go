package target

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope is the level a built-in Target resolves at; custom Targets carry
// ScopeCustom because they directly name a Skills container.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeCustom  Scope = "custom"
)

// ResolveOptions carries the process state a resolution reads. Zero-value
// function fields fall back to the os implementation so production call
// sites need no ceremony; tests inject them for determinism.
type ResolveOptions struct {
	Home         string
	LookupEnv    func(string) (string, bool)
	EvalSymlinks func(string) (string, error)
	Stat         func(string) (os.FileInfo, error)
}

func (o ResolveOptions) withDefaults() ResolveOptions {
	if o.LookupEnv == nil {
		o.LookupEnv = os.LookupEnv
	}
	if o.EvalSymlinks == nil {
		o.EvalSymlinks = filepath.EvalSymlinks
	}
	if o.Stat == nil {
		o.Stat = os.Stat
	}
	return o
}

// ResolvedTarget is the fixed physical identity of one Target registration:
// the normalized absolute path with every existing prefix resolved through
// symbolic links, plus the explanatory creation metadata.
type ResolvedTarget struct {
	Path        string
	Adapter     string
	Scope       Scope
	ProjectRoot string
}

// ResolveUser resolves one built-in adapter's user-level Skills container
// from HOME and any adapter-specific override.
func ResolveUser(a Adapter, o ResolveOptions) (ResolvedTarget, error) {
	o = o.withDefaults()
	if o.Home == "" {
		return ResolvedTarget{}, fmt.Errorf("HOME must be set to resolve a user Target")
	}
	base, err := a.overrideBase(o.Home, o.LookupEnv)
	if err != nil {
		return ResolvedTarget{}, err
	}
	path, err := finalizePath(filepath.Join(base, a.UserSuffix), o)
	if err != nil {
		return ResolvedTarget{}, err
	}
	return ResolvedTarget{Path: path, Adapter: a.Key, Scope: ScopeUser}, nil
}

// ResolveProject resolves one built-in adapter's project-level Skills
// container under an explicit, existing project root directory. The root's
// symbolic links are resolved before the Target path is fixed; the
// adapter's Skills directory need not exist yet.
func ResolveProject(a Adapter, projectRoot string, o ResolveOptions) (ResolvedTarget, error) {
	o = o.withDefaults()
	if projectRoot == "" {
		return ResolvedTarget{}, fmt.Errorf("a project Target requires an explicit project root directory")
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return ResolvedTarget{}, err
	}
	root, err := o.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedTarget{}, fmt.Errorf("project root %s does not exist", abs)
		}
		return ResolvedTarget{}, err
	}
	if fi, err := o.Stat(root); err != nil {
		return ResolvedTarget{}, err
	} else if !fi.IsDir() {
		return ResolvedTarget{}, fmt.Errorf("project root %s is not a directory", root)
	}
	path, err := finalizePath(filepath.Join(root, a.ProjectSuffix), o)
	if err != nil {
		return ResolvedTarget{}, err
	}
	return ResolvedTarget{Path: path, Adapter: a.Key, Scope: ScopeProject, ProjectRoot: root}, nil
}

// ResolveCustom resolves one custom Target from an operator-named Skills
// container: an absolute path or a leading ~/ path. The container is not a
// project root and has no hidden adapter suffix.
func ResolveCustom(input string, o ResolveOptions) (ResolvedTarget, error) {
	o = o.withDefaults()
	switch {
	case strings.HasPrefix(input, "~") && input != "~" && !strings.HasPrefix(input, "~/"):
		return ResolvedTarget{}, fmt.Errorf("custom Target path %q: only a leading ~/ is expanded, not ~name", input)
	case strings.ContainsAny(input, "$*?["):
		return ResolvedTarget{}, fmt.Errorf("custom Target path %q must not contain environment variables or glob patterns", input)
	}
	p := input
	if input == "~" || strings.HasPrefix(input, "~/") {
		if o.Home == "" {
			return ResolvedTarget{}, fmt.Errorf("HOME must be set to expand %q", input)
		}
		rest := strings.TrimPrefix(input, "~")
		if strings.HasPrefix(rest, "/") {
			rest = rest[1:]
		}
		p = filepath.Join(o.Home, rest)
	} else if !filepath.IsAbs(input) {
		return ResolvedTarget{}, fmt.Errorf("custom Target path must be absolute or start with ~/: %q", input)
	}
	path, err := finalizePath(p, o)
	if err != nil {
		return ResolvedTarget{}, err
	}
	return ResolvedTarget{Path: path, Adapter: "custom", Scope: ScopeCustom}, nil
}

// finalizePath turns an absolute candidate into the fixed physical
// identity: clean it, resolve every existing prefix through symbolic
// links, reject the filesystem root, and reject an existing non-directory
// container. The container itself may be absent; registration performs no
// filesystem mutation.
func finalizePath(p string, o ResolveOptions) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if isFilesystemRoot(abs) {
		return "", fmt.Errorf("the filesystem root cannot be a Target")
	}
	resolved, err := resolveExistingPrefix(abs, o.EvalSymlinks)
	if err != nil {
		return "", err
	}
	if fi, err := o.Stat(resolved); err == nil {
		if !fi.IsDir() {
			return "", fmt.Errorf("Target path %s exists and is not a directory", resolved)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspecting Target path %s: %v", resolved, err)
	}
	return resolved, nil
}

// resolveExistingPrefix resolves the longest existing prefix of path
// through symbolic links and rejoins the not-yet-existing remainder
// literally, so the stored identity is the physical location registration
// observed.
func resolveExistingPrefix(path string, eval func(string) (string, error)) (string, error) {
	suffix := ""
	cur := path
	for {
		resolved, err := eval(cur)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return path, nil // the filesystem root always exists
		}
		suffix = filepath.Join(filepath.Base(cur), suffix)
		cur = parent
	}
}

func isFilesystemRoot(p string) bool {
	return p == string(filepath.Separator)
}

// CheckStoreDisjoint verifies that a resolved Target path and the Skill
// Store are neither equal nor in an ancestor/descendant relationship
// (decision 04). The Store's existing prefix is resolved through symbolic
// links the same way the Target's was.
func CheckStoreDisjoint(targetPath, storePath string, o ResolveOptions) error {
	o = o.withDefaults()
	abs, err := filepath.Abs(storePath)
	if err != nil {
		return err
	}
	resolved, err := resolveExistingPrefix(filepath.Clean(abs), o.EvalSymlinks)
	if err != nil {
		return err
	}
	if pathsOverlap(targetPath, resolved) {
		return fmt.Errorf("Target path %s must be disjoint from the Skill Store %s", targetPath, resolved)
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	return a == b || within(a, b) || within(b, a)
}

// within reports whether child is parent itself or inside it.
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

// CompatibleAdapters derives which built-in adapters share one resolved
// Skills path: the user-level resolution for a user Target, or the
// project-level resolution under the recorded project root for a project
// Target. A shared Target does not persist Agent ownership, so this list
// is derived, not stored (decision 04). An adapter whose current
// resolution fails simply does not match.
func CompatibleAdapters(path string, scope Scope, projectRoot string, o ResolveOptions) []string {
	o = o.withDefaults()
	var out []string
	for _, a := range adapters {
		var rt ResolvedTarget
		var err error
		switch scope {
		case ScopeUser:
			rt, err = ResolveUser(a, o)
		case ScopeProject:
			rt, err = ResolveProject(a, projectRoot, o)
		default:
			continue
		}
		if err != nil {
			continue
		}
		if rt.Path == path {
			out = append(out, a.Key)
		}
	}
	return out
}
