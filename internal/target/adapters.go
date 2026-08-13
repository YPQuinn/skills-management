// Package target owns the built-in Target Adapter table and the pure
// resolution, safety, and detection functions around it. A Target Adapter
// is a path-discovery aid, not a Target's identity or owner (decision 04).
package target

import (
	"fmt"
	"path/filepath"
)

// Adapter is one built-in Target Adapter: the path rules for user-level
// and project-level Skills containers plus the advisory installation
// detection signals. Universal is the ecosystem-wide fallback and is not
// one concrete detectable Agent.
//
// For an adapter with OverrideEnv, the base directory is the non-empty
// override value, or home/DefaultBase when the override is unset or empty
// (XDG_CONFIG_HOME defaults to ~/.config, not ~). UserSuffix and
// ConfigRoot are then appended to that base.
type Adapter struct {
	Key           string
	Name          string
	Executable    string
	MacApps       []string
	OverrideEnv   string
	DefaultBase   string
	ConfigRoot    string
	ProjectSuffix string
	UserSuffix    string
}

// adapters is the curated, versioned MVP table (decision 04).
var adapters = []Adapter{
	{Key: "universal", Name: "Universal", ProjectSuffix: ".agents/skills", UserSuffix: ".agents/skills"},
	{
		Key: "claude-code", Name: "Claude Code", Executable: "claude",
		OverrideEnv: "CLAUDE_CONFIG_DIR", DefaultBase: ".claude",
		ProjectSuffix: ".claude/skills", UserSuffix: "skills",
	},
	{
		Key: "codex", Name: "Codex", Executable: "codex", MacApps: []string{"ChatGPT.app", "Codex.app"},
		OverrideEnv: "CODEX_HOME", DefaultBase: ".codex",
		ProjectSuffix: ".agents/skills", UserSuffix: "skills",
	},
	{
		Key: "cursor", Name: "Cursor", Executable: "cursor", MacApps: []string{"Cursor.app"},
		ConfigRoot: ".cursor", ProjectSuffix: ".agents/skills", UserSuffix: ".cursor/skills",
	},
	{
		Key: "gemini-cli", Name: "Gemini CLI", Executable: "gemini",
		ConfigRoot: ".gemini", ProjectSuffix: ".agents/skills", UserSuffix: ".gemini/skills",
	},
	{
		Key: "opencode", Name: "OpenCode", Executable: "opencode",
		OverrideEnv: "XDG_CONFIG_HOME", DefaultBase: ".config", ConfigRoot: "opencode",
		ProjectSuffix: ".agents/skills", UserSuffix: "opencode/skills",
	},
	{
		Key: "pi", Name: "Pi", Executable: "pi",
		ConfigRoot: ".pi/agent", ProjectSuffix: ".pi/skills", UserSuffix: ".pi/agent/skills",
	},
	{
		Key: "github-copilot", Name: "GitHub Copilot", Executable: "copilot",
		ConfigRoot: ".copilot", ProjectSuffix: ".agents/skills", UserSuffix: ".copilot/skills",
	},
}

// Adapters returns a copy of the built-in adapter table in canonical order.
func Adapters() []Adapter {
	out := make([]Adapter, len(adapters))
	copy(out, adapters)
	return out
}

// ByKey returns the built-in adapter with the given key.
func ByKey(key string) (Adapter, bool) {
	for _, a := range adapters {
		if a.Key == key {
			return a, true
		}
	}
	return Adapter{}, false
}

// overrideBase resolves the base directory an adapter derives its user
// path and configuration root from: the non-empty override value when the
// adapter has one and it is set, otherwise Home (plus DefaultBase when the
// adapter declares one, e.g. XDG_CONFIG_HOME defaults to ~/.config). A
// non-empty relative override is an error rather than a reason to silently
// use the default (decision 04).
func (a Adapter) overrideBase(home string, lookupEnv func(string) (string, bool)) (string, error) {
	base := home
	if a.OverrideEnv != "" {
		if v, ok := lookupEnv(a.OverrideEnv); ok && v != "" {
			if !filepath.IsAbs(v) {
				return "", fmt.Errorf("%s=%q must be an absolute path", a.OverrideEnv, v)
			}
			base = v
		} else {
			base = filepath.Join(home, a.DefaultBase)
		}
	}
	return base, nil
}

// configRoot resolves one adapter's user configuration root for detection.
func (a Adapter) configRoot(home string, lookupEnv func(string) (string, bool)) (string, error) {
	base, err := a.overrideBase(home, lookupEnv)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, a.ConfigRoot), nil
}
