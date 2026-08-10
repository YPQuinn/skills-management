package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestLoopbackHostAndOrigin(t *testing.T) {
	ts := startServer(t)
	port := strings.TrimPrefix(ts.baseURL, "http://127.0.0.1:")
	goodHost := "127.0.0.1:" + port

	cases := []struct {
		name, host, origin string
		wantStatus         int
	}{
		{"foreign host", "evil.com:1234", "", http.StatusForbidden},
		{"wrong loopback port", "127.0.0.1:1", "", http.StatusForbidden},
		{"localhost is not the exact loopback host", "localhost:" + port, "", http.StatusForbidden},
		{"good host, evil origin", goodHost, "http://evil.com", http.StatusForbidden},
		{"good host, origin wrong port", goodHost, "http://127.0.0.1:1", http.StatusForbidden},
		{"good host, origin trailing slash", goodHost, "http://127.0.0.1:" + port + "/", http.StatusForbidden},
		{"good host, exact origin", goodHost, "http://127.0.0.1:" + port, http.StatusOK},
		{"good host, no origin", goodHost, "", http.StatusOK},
	}
	for _, c := range cases {
		resp := ts.do(t, "GET", "/api/v1/status", "", "", c.host, c.origin)
		if resp.StatusCode != c.wantStatus {
			t.Errorf("%s: got %d, want %d (body %s)", c.name, resp.StatusCode, c.wantStatus, readBody(t, resp))
			continue
		}
		if c.wantStatus == http.StatusForbidden {
			if env := decodeError(t, readBody(t, resp)); env.Error.Code != "forbidden" {
				t.Errorf("%s: got code %q, want forbidden", c.name, env.Error.Code)
			}
		}
	}
}

func TestHostValidationCoversAllRoutes(t *testing.T) {
	ts := startServer(t)
	port := strings.TrimPrefix(ts.baseURL, "http://127.0.0.1:")
	goodHost := "127.0.0.1:" + port
	evilHost := "evil.com:1234"

	// hostile Host is rejected on the SPA root, setup, and deep links
	for _, path := range []string{"/", "/setup", "/skills/example-skill", "/targets/editor"} {
		resp := ts.do(t, "GET", path, "", "", evilHost, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s with hostile Host: got %d, want 403", path, resp.StatusCode)
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != "forbidden" {
			t.Errorf("GET %s hostile Host code: got %q", path, env.Error.Code)
		}
	}

	// hostile Host on a real hashed asset
	root := ts.do(t, "GET", "/", "", "", goodHost, "")
	html := readBody(t, root)
	start := strings.Index(html, `src="`)
	if start < 0 {
		t.Fatal("index.html has no script src")
	}
	asset := html[start+len(`src="`) : strings.Index(html[start+len(`src="`):], `"`)+start+len(`src="`)]
	resp := ts.do(t, "GET", asset, "", "", evilHost, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET %s with hostile Host: got %d, want 403", asset, resp.StatusCode)
	}
	readBody(t, resp)

	// hostile Host beats Content-Type checks on the setup mutation
	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "", evilHost, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST setup with hostile Host: got %d, want 403", resp.StatusCode)
	}
	readBody(t, resp)

	// the exact loopback Host still reaches the SPA
	resp = ts.do(t, "GET", "/", "", "", goodHost, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), `<div id="root">`) {
		t.Fatalf("GET / with exact loopback Host: got %d", resp.StatusCode)
	}

	// a hostile Origin is rejected on the SPA tree too
	resp = ts.do(t, "GET", "/", "", "", goodHost, "http://evil.com")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET / with hostile Origin: got %d, want 403", resp.StatusCode)
	}
	readBody(t, resp)
}

func TestAPIUnknownRouteOrdering(t *testing.T) {
	ts := startServer(t)

	// an unknown API POST without Content-Type is a JSON 404, not 415
	resp := ts.do(t, "POST", "/api/v1/unknown", `{}`, "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown POST without Content-Type: got %d, want 404", resp.StatusCode)
	}
	if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
		t.Fatalf("unknown POST code: got %q", env.Error.Code)
	}

	// a wrong method on a known API path is a JSON 404 too
	for _, method := range []string{"PUT", "DELETE"} {
		resp := ts.do(t, method, "/api/v1/status", `{}`, "", "", "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s /api/v1/status: got %d, want 404", method, resp.StatusCode)
			continue
		}
		if env := decodeError(t, readBody(t, resp)); env.Error.Code != "not_found" {
			t.Errorf("%s /api/v1/status code: got %q", method, env.Error.Code)
		}
	}

	// known mutation routes still enforce application/json
	resp = ts.do(t, "POST", "/api/v1/setup", `{}`, "", "", "")
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("setup without Content-Type: got %d, want 415", resp.StatusCode)
	}
	readBody(t, resp)
}

func TestAPIIsolationAndHeaders(t *testing.T) {
	ts := startServer(t)

	resp := ts.do(t, "GET", "/api/v1/unknown", "", "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown API route: got %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("API 404 content type: got %q", ct)
	}
	if env := decodeError(t, body); env.Error.Code != "not_found" {
		t.Fatalf("API 404 code: got %q", env.Error.Code)
	}
	if strings.Contains(body, "<html") || strings.Contains(body, "root") {
		t.Fatalf("API 404 leaked the SPA: %s", body)
	}

	resp = ts.do(t, "POST", "/api/v1/unknown", `{}`, "application/json", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST unknown API route: got %d", resp.StatusCode)
	}
	readBody(t, resp)

	// stable security headers on both the API and the SPA
	for _, path := range []string{"/api/v1/status", "/"} {
		resp := ts.do(t, "GET", path, "", "", "", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: got %d", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Security-Policy"); got != contentSecurityPolicy {
			t.Errorf("%s CSP: got %q", path, got)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s nosniff: got %q", path, got)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("%s referrer policy: got %q", path, got)
		}
		readBody(t, resp)
	}

	// SPA deep link through the mounted router
	resp = ts.do(t, "GET", "/skills/example-skill", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SPA deep link: got %d", resp.StatusCode)
	}
	body = readBody(t, resp)
	if !strings.Contains(body, `<div id="root">`) {
		t.Fatalf("deep link did not receive the SPA shell")
	}
}
