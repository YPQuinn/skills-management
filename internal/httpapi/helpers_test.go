package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"skillctl/internal/bootstrap"
	"strings"
	"testing"
	"time"
)

type testServer struct {
	baseURL string
	bm      *bootstrap.Manager
	cfgPath string
}

// startServer binds a random loopback port and serves the real router, so
// Host/Origin validation runs against the actual bound port.
func startServer(t *testing.T) *testServer {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), ".skillctl", "config.toml")
	bm := bootstrap.New(cfgPath)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: New(bm, port).Handler()}
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	return &testServer{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		bm:      bm,
		cfgPath: cfgPath,
	}
}

func (ts *testServer) do(t *testing.T, method, path, body, contentType, host, origin string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.baseURL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeStatus(t *testing.T, body string) statusResponse {
	t.Helper()
	var st statusResponse
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return st
}

func decodeError(t *testing.T, body string) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decoding error envelope %q: %v", body, err)
	}
	return env
}
