package target

import (
	"reflect"
	"testing"
)

// TestAdapterTable pins the curated MVP table (decision 04): exactly eight
// adapters with the documented executable, override, and suffix contract.
func TestAdapterTable(t *testing.T) {
	all := Adapters()
	if len(all) != 8 {
		t.Fatalf("adapter count: got %d, want 8", len(all))
	}
	want := map[string]Adapter{
		"universal":      {Key: "universal", ProjectSuffix: ".agents/skills", UserSuffix: ".agents/skills"},
		"claude-code":    {Key: "claude-code", Executable: "claude", OverrideEnv: "CLAUDE_CONFIG_DIR", DefaultBase: ".claude", ProjectSuffix: ".claude/skills", UserSuffix: "skills"},
		"codex":          {Key: "codex", Executable: "codex", OverrideEnv: "CODEX_HOME", DefaultBase: ".codex", ProjectSuffix: ".agents/skills", UserSuffix: "skills"},
		"cursor":         {Key: "cursor", Executable: "cursor", ConfigRoot: ".cursor", ProjectSuffix: ".agents/skills", UserSuffix: ".cursor/skills"},
		"gemini-cli":     {Key: "gemini-cli", Executable: "gemini", ConfigRoot: ".gemini", ProjectSuffix: ".agents/skills", UserSuffix: ".gemini/skills"},
		"opencode":       {Key: "opencode", Executable: "opencode", OverrideEnv: "XDG_CONFIG_HOME", DefaultBase: ".config", ConfigRoot: "opencode", ProjectSuffix: ".agents/skills", UserSuffix: "opencode/skills"},
		"pi":             {Key: "pi", Executable: "pi", ConfigRoot: ".pi/agent", ProjectSuffix: ".pi/skills", UserSuffix: ".pi/agent/skills"},
		"github-copilot": {Key: "github-copilot", Executable: "copilot", ConfigRoot: ".copilot", ProjectSuffix: ".agents/skills", UserSuffix: ".copilot/skills"},
	}
	for _, a := range all {
		w, ok := want[a.Key]
		if !ok {
			t.Fatalf("unexpected adapter %q", a.Key)
		}
		if a.Executable != w.Executable || a.OverrideEnv != w.OverrideEnv || a.DefaultBase != w.DefaultBase || a.ConfigRoot != w.ConfigRoot ||
			a.ProjectSuffix != w.ProjectSuffix || a.UserSuffix != w.UserSuffix {
			t.Fatalf("adapter %q: got %+v, want %+v", a.Key, a, w)
		}
		delete(want, a.Key)
	}
	if len(want) != 0 {
		t.Fatalf("missing adapters: %v", want)
	}
	if a, ok := ByKey("pi"); !ok || a.Executable != "pi" {
		t.Fatalf("ByKey(pi): %+v, %v", a, ok)
	}
	if _, ok := ByKey("nope"); ok {
		t.Fatal("ByKey(nope) must not match")
	}
}

// TestCodexMacApps pins the macOS application evidence names.
func TestCodexMacApps(t *testing.T) {
	a, ok := ByKey("codex")
	if !ok {
		t.Fatal("codex missing")
	}
	if !reflect.DeepEqual(a.MacApps, []string{"ChatGPT.app", "Codex.app"}) {
		t.Fatalf("codex apps: %v", a.MacApps)
	}
	c, _ := ByKey("cursor")
	if !reflect.DeepEqual(c.MacApps, []string{"Cursor.app"}) {
		t.Fatalf("cursor apps: %v", c.MacApps)
	}
}
