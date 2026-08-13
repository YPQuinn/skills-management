package target

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFS is a tiny in-memory file system for detection: files with modes,
// and directories with entries.
type fakeFS struct {
	files   map[string]os.FileInfo
	dirs    map[string][]fs.DirEntry
	readErr map[string]error
}

type fakeEntry struct {
	name  string
	isDir bool
}

func (f fakeEntry) Name() string               { return f.name }
func (f fakeEntry) IsDir() bool                { return f.isDir }
func (f fakeEntry) Type() fs.FileMode          { return 0 }
func (f fakeEntry) Info() (fs.FileInfo, error) { return nil, nil }

type fakeInfo struct {
	name string
	dir  bool
	mode fs.FileMode
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return 0 }
func (i fakeInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeInfo) ModTime() time.Time { return time.Time{} }
func (i fakeInfo) IsDir() bool        { return i.dir }
func (i fakeInfo) Sys() any           { return nil }

func newFakeFS() *fakeFS {
	return &fakeFS{
		files:   map[string]os.FileInfo{},
		dirs:    map[string][]fs.DirEntry{},
		readErr: map[string]error{},
	}
}

func (f *fakeFS) addFile(path string, mode fs.FileMode) {
	f.files[path] = fakeInfo{name: filepath.Base(path), mode: mode}
}

func (f *fakeFS) addDir(path string, entries ...string) {
	f.files[path] = fakeInfo{name: filepath.Base(path), dir: true}
	dir := f.dirs[path]
	for _, e := range entries {
		dir = append(dir, fakeEntry{name: e, isDir: true})
	}
	f.dirs[path] = dir
}

func (f *fakeFS) stat(path string) (os.FileInfo, error) {
	if err, ok := f.readErr[path]; ok {
		return nil, err
	}
	if fi, ok := f.files[path]; ok {
		return fi, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeFS) readDir(path string) ([]fs.DirEntry, error) {
	if err, ok := f.readErr[path]; ok {
		return nil, err
	}
	entries, ok := f.dirs[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return entries, nil
}

func detectOptions(home string, f *fakeFS) DetectionOptions {
	return DetectionOptions{
		Home:      home,
		PATH:      "/usr/bin:/opt/bin",
		LookupEnv: func(string) (string, bool) { return "", false },
		AppDirs:   []string{"/Applications"},
		Stat:      f.stat,
		ReadDir:   f.readDir,
	}
}

// TestDetectExecutable pins the PATH signal.
func TestDetectExecutable(t *testing.T) {
	f := newFakeFS()
	f.addFile("/usr/bin/pi", 0o755)
	a, _ := ByKey("pi")
	d := Detect(a, detectOptions(t.TempDir(), f))
	if d.Status != StatusDetected || len(d.Evidence) != 1 || !strings.Contains(d.Evidence[0], "/usr/bin/pi") {
		t.Fatalf("detection: %+v", d)
	}
	if d.DetectedAt.IsZero() {
		t.Fatal("detection time missing")
	}
	// a non-executable file is not evidence
	f = newFakeFS()
	f.addFile("/usr/bin/pi", 0o644)
	if d := Detect(a, detectOptions(t.TempDir(), f)); d.Status != StatusNotDetected {
		t.Fatalf("non-executable: %+v", d)
	}
	// empty PATH finds nothing
	f = newFakeFS()
	if d := Detect(a, DetectionOptions{Home: t.TempDir(), PATH: "", Stat: f.stat, ReadDir: f.readDir, LookupEnv: func(string) (string, bool) { return "", false }}); d.Status != StatusNotDetected {
		t.Fatalf("empty PATH: %+v", d)
	}
}

// TestDetectMacApp pins the macOS application evidence.
func TestDetectMacApp(t *testing.T) {
	f := newFakeFS()
	f.addDir("/Applications/Cursor.app")
	a, _ := ByKey("cursor")
	d := Detect(a, detectOptions(t.TempDir(), f))
	if d.Status != StatusDetected || len(d.Evidence) != 1 || !strings.Contains(d.Evidence[0], "Cursor.app") {
		t.Fatalf("detection: %+v", d)
	}
}

// TestDetectConfigRoot pins the configuration-root signal: any entry other
// than a skills directory is evidence, while a skills-only or missing root
// is not.
func TestDetectConfigRoot(t *testing.T) {
	home := t.TempDir()
	a, _ := ByKey("pi")
	root := filepath.Join(home, ".pi", "agent")

	f := newFakeFS()
	f.addDir(root, "settings.json", "skills")
	if d := Detect(a, detectOptions(home, f)); d.Status != StatusDetected {
		t.Fatalf("config with settings: %+v", d)
	}

	f = newFakeFS()
	f.addDir(root, "skills")
	if d := Detect(a, detectOptions(home, f)); d.Status != StatusNotDetected {
		t.Fatalf("skills-only config is not evidence: %+v", d)
	}

	f = newFakeFS()
	if d := Detect(a, detectOptions(home, f)); d.Status != StatusNotDetected {
		t.Fatalf("missing config: %+v", d)
	}
}

// TestDetectUnknown pins the inspection-failure rule: a read error
// produces unknown rather than a guess.
func TestDetectUnknown(t *testing.T) {
	home := t.TempDir()
	a, _ := ByKey("pi")
	f := newFakeFS()
	f.readErr[filepath.Join(home, ".pi", "agent")] = fmt.Errorf("permission denied")
	if d := Detect(a, detectOptions(home, f)); d.Status != StatusUnknown {
		t.Fatalf("read error: %+v", d)
	}
	// a non-empty relative override is an inspection failure too
	f = newFakeFS()
	opts := detectOptions(home, f)
	opts.LookupEnv = func(k string) (string, bool) {
		if k == "XDG_CONFIG_HOME" {
			return "relative", true
		}
		return "", false
	}
	oc, _ := ByKey("opencode")
	if d := Detect(oc, opts); d.Status != StatusUnknown {
		t.Fatalf("relative override: %+v", d)
	}
}

// TestDetectNotApplicable pins that Universal (and therefore custom
// registration, which has no adapter) reports not_applicable.
func TestDetectNotApplicable(t *testing.T) {
	a, _ := ByKey("universal")
	d := Detect(a, detectOptions(t.TempDir(), newFakeFS()))
	if d.Status != StatusNotApplicable {
		t.Fatalf("universal: %+v", d)
	}
}

// TestDetectAllCoversEveryAdapter pins that the on-demand sweep reports
// exactly one result per built-in adapter in canonical order.
func TestDetectAllCoversEveryAdapter(t *testing.T) {
	all := DetectAll(detectOptions(t.TempDir(), newFakeFS()))
	if len(all) != len(Adapters()) {
		t.Fatalf("detections: %d, want %d", len(all), len(Adapters()))
	}
	for i, a := range Adapters() {
		if all[i].Adapter != a.Key {
			t.Fatalf("detection %d: %s, want %s", i, all[i].Adapter, a.Key)
		}
	}
}
