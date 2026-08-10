// Package webui embeds the compiled Appica SPA and serves it: exact embedded
// files first, SPA fallback only for missing extensionless GET/HEAD paths
// that look like client routes. Missing assets, missing paths with
// extensions, API paths, directories, and unsupported methods never receive
// the SPA shell.
package webui

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA from the request path.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		http.Error(w, "Web UI is not built", http.StatusInternalServerError)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}

	// "skills/" is not a valid fs path, so trailing-slash client routes
	// would 404 before the SPA fallback. Stat the slash-stripped name so
	// "/skills/" matches "/skills", while directories ("/assets/") and
	// extension-bearing paths ("/missing.js/", "/index.html/") never
	// receive the shell.
	statName := strings.TrimSuffix(name, "/")

	info, err := fs.Stat(dist, statName)
	switch {
	case err == nil && !info.IsDir() && statName == name:
		serveFile(w, r, dist, statName)
	case err == nil: // directories and trailing-slash file paths are not client routes
		http.NotFound(w, r)
	case errors.Is(err, fs.ErrNotExist):
		if strings.HasPrefix(name, "assets/") || strings.Contains(filepath.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, dist, "index.html")
	default: // invalid paths and unexpected stat failures never get the SPA
		http.NotFound(w, r)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, dist fs.FS, name string) {
	data, err := fs.ReadFile(dist, name)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
