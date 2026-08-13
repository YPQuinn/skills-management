package target

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DetectionStatus is the advisory on-demand result of one adapter's
// installation detection (decision 04).
type DetectionStatus string

const (
	StatusDetected      DetectionStatus = "detected"
	StatusNotDetected   DetectionStatus = "not_detected"
	StatusUnknown       DetectionStatus = "unknown"
	StatusNotApplicable DetectionStatus = "not_applicable"
)

// Detection is one adapter's detection result with the evidence that
// produced it and the detection time. Detection is never stored.
type Detection struct {
	Adapter    string          `json:"adapter"`
	Status     DetectionStatus `json:"status"`
	Evidence   []string        `json:"evidence"`
	DetectedAt time.Time       `json:"detected_at"`
}

// DetectionOptions carries the environment a detection reads. Zero-value
// function fields fall back to the os implementation.
type DetectionOptions struct {
	Home      string
	PATH      string
	LookupEnv func(string) (string, bool)
	AppDirs   []string
	Stat      func(string) (os.FileInfo, error)
	ReadDir   func(string) ([]os.DirEntry, error)
}

func (o DetectionOptions) withDefaults() DetectionOptions {
	if o.LookupEnv == nil {
		o.LookupEnv = os.LookupEnv
	}
	if o.Stat == nil {
		o.Stat = os.Stat
	}
	if o.ReadDir == nil {
		o.ReadDir = os.ReadDir
	}
	return o
}

// DefaultDetectionOptions reads the process environment for an on-demand
// detection: HOME, PATH, the standard and per-user Applications
// directories, and the real lookup/stat/read functions.
func DefaultDetectionOptions() DetectionOptions {
	home, _ := os.UserHomeDir()
	return DetectionOptions{
		Home:      home,
		PATH:      os.Getenv("PATH"),
		LookupEnv: os.LookupEnv,
		AppDirs:   []string{"/Applications", filepath.Join(home, "Applications")},
		Stat:      os.Stat,
		ReadDir:   os.ReadDir,
	}
}

// Detect inspects one adapter read-only on demand. An adapter is detected
// when its executable is on PATH, a known macOS application entry exists,
// or its user configuration root contains any entry other than a skills
// directory. Inspection failures produce unknown; Universal has no
// concrete detectable Agent and reports not_applicable.
func Detect(a Adapter, o DetectionOptions) Detection {
	o = o.withDefaults()
	d := Detection{Adapter: a.Key, Evidence: []string{}, DetectedAt: time.Now().UTC()}
	if a.Executable == "" {
		d.Status = StatusNotApplicable
		return d
	}
	unknown := false

	for _, dir := range filepath.SplitList(o.PATH) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, a.Executable)
		fi, err := o.Stat(p)
		if err == nil {
			if fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
				d.Evidence = append(d.Evidence, "executable "+p)
				break
			}
			continue
		}
		if !os.IsNotExist(err) {
			unknown = true
			d.Evidence = append(d.Evidence, fmt.Sprintf("cannot inspect %s: %v", p, err))
		}
	}

	for _, app := range a.MacApps {
		for _, dir := range o.AppDirs {
			if dir == "" {
				continue
			}
			p := filepath.Join(dir, app)
			fi, err := o.Stat(p)
			if err == nil {
				if fi.IsDir() {
					d.Evidence = append(d.Evidence, "application "+p)
				}
				break
			}
			if !os.IsNotExist(err) {
				unknown = true
				d.Evidence = append(d.Evidence, fmt.Sprintf("cannot inspect %s: %v", p, err))
			}
		}
	}

	if a.ConfigRoot != "" {
		if o.Home == "" {
			unknown = true
			d.Evidence = append(d.Evidence, "HOME is not set")
		} else {
			root, err := a.configRoot(o.Home, o.LookupEnv)
			if err != nil {
				unknown = true
				d.Evidence = append(d.Evidence, err.Error())
			} else if entries, err := o.ReadDir(root); err == nil {
				var names []string
				for _, e := range entries {
					if e.Name() == "skills" {
						continue
					}
					names = append(names, e.Name())
					if len(names) == 3 {
						break
					}
				}
				if len(names) > 0 {
					d.Evidence = append(d.Evidence, fmt.Sprintf("configuration %s contains %s", root, strings.Join(names, ", ")))
				}
			} else if !os.IsNotExist(err) {
				unknown = true
				d.Evidence = append(d.Evidence, fmt.Sprintf("cannot read %s: %v", root, err))
			}
		}
	}

	switch {
	case unknown:
		d.Status = StatusUnknown
	case len(d.Evidence) > 0:
		d.Status = StatusDetected
	default:
		d.Status = StatusNotDetected
	}
	return d
}

// DetectAll runs Detect for every built-in adapter in canonical order.
func DetectAll(o DetectionOptions) []Detection {
	o = o.withDefaults()
	out := make([]Detection, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, Detect(a, o))
	}
	return out
}
