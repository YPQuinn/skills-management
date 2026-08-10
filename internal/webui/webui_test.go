package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const rootMarker = `<div id="root">`

func request(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	return rec
}

func TestServesExactEmbeddedFiles(t *testing.T) {
	rec := request(t, "GET", "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET / content type: got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), rootMarker) {
		t.Fatalf("GET / did not return the SPA shell")
	}

	rec = request(t, "GET", "/index.html")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), rootMarker) {
		t.Fatalf("GET /index.html: got %d", rec.Code)
	}

	rec = request(t, "GET", "/favicon.svg")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.svg: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("favicon content type: got %q", ct)
	}

	// a hashed asset referenced from index.html must be served exactly
	rec = request(t, "GET", "/")
	html := rec.Body.String()
	start := strings.Index(html, `src="`)
	if start < 0 {
		t.Fatal("index.html has no script src")
	}
	asset := html[start+len(`src="`) : strings.Index(html[start+len(`src="`):], `"`)+start+len(`src="`)]
	rec = request(t, "GET", asset)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: got %d", asset, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("asset content type: got %q", ct)
	}
}

func TestClientRouteFallback(t *testing.T) {
	for _, path := range []string{"/setup", "/skills", "/skills/example-skill", "/targets/editor"} {
		rec := request(t, "GET", path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), rootMarker) {
			t.Errorf("GET %s did not fall back to the SPA shell", path)
		}
	}

	rec := request(t, "HEAD", "/skills/example-skill")
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD deep link: got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD returned a body (%d bytes)", rec.Body.Len())
	}
}

func TestTrailingSlashClientRoutes(t *testing.T) {
	for _, path := range []string{"/setup/", "/skills/", "/skills/example-skill/", "/targets/editor/"} {
		rec := request(t, "GET", path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), rootMarker) {
			t.Errorf("GET %s did not fall back to the SPA shell", path)
		}
	}

	rec := request(t, "HEAD", "/skills/example-skill/")
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD trailing-slash deep link: got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD returned a body (%d bytes)", rec.Body.Len())
	}
}

func TestNeverReceivesSPA(t *testing.T) {
	cases := []struct {
		method, path string
		wantStatus   int
	}{
		{"GET", "/assets/missing.js", http.StatusNotFound},
		{"GET", "/assets/missing.css", http.StatusNotFound},
		{"GET", "/assets/noext", http.StatusNotFound},
		{"GET", "/missing.js", http.StatusNotFound},
		{"GET", "/missing.js/", http.StatusNotFound},
		{"GET", "/index.html/", http.StatusNotFound},
		{"GET", "/favicon.svg/", http.StatusNotFound},
		{"GET", "/styles.old.css", http.StatusNotFound},
		{"GET", "/api/v1/unknown", http.StatusNotFound},
		{"GET", "/api/v1/unknown/", http.StatusNotFound},
		{"GET", "/api/unknown", http.StatusNotFound},
		{"GET", "/api", http.StatusNotFound},
		{"GET", "/assets", http.StatusNotFound},
		{"GET", "/assets/", http.StatusNotFound},
		{"POST", "/", http.StatusMethodNotAllowed},
		{"PUT", "/skills", http.StatusMethodNotAllowed},
		{"DELETE", "/setup", http.StatusMethodNotAllowed},
		{"POST", "/skills/", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		rec := request(t, c.method, c.path)
		if rec.Code != c.wantStatus {
			t.Errorf("%s %s: got %d, want %d", c.method, c.path, rec.Code, c.wantStatus)
			continue
		}
		if strings.Contains(rec.Body.String(), rootMarker) {
			t.Errorf("%s %s received the SPA shell", c.method, c.path)
		}
	}

	rec := request(t, "POST", "/")
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("POST / Allow header: got %q", allow)
	}
}

func TestInvalidPathNeverServesSPA(t *testing.T) {
	req := httptest.NewRequest("GET", "/../secret", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /../secret: got %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), rootMarker) {
		t.Fatal("traversal path received the SPA shell")
	}
}

func TestBodyIsShellNotFileListing(t *testing.T) {
	rec := request(t, "GET", "/assets")
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "index-") {
		t.Fatal("directory request leaked a file listing")
	}
}
