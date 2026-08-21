// Package main contains the separate-process end-to-end tests for skillctl.
// They build a temporary binary once per test run, isolate HOME per child
// process, use port 0 everywhere, and never sleep-poll: readiness is
// signalled by the URL line on stdout (printed after the listener is bound,
// so connections queue in the kernel backlog).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type uiProcess struct {
	cmd      *exec.Cmd
	url      string
	done     chan error
	stopOnce sync.Once
}

func startUI(t *testing.T, home string) *uiProcess {
	t.Helper()
	cmd := exec.Command(testBinary(t), "ui", "--port", "0", "--no-open")
	cmd.Env = append(os.Environ(), "HOME="+home)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	up := &uiProcess{cmd: cmd, done: done}
	t.Cleanup(func() { up.stop(t) })
	up.url = waitForURL(t, stdout, &stderr)
	return up
}

// stop terminates the server gracefully and waits for it to exit. It is
// idempotent so test-body shutdown and Cleanup cannot race on Wait.
// Signal/Kill errors other than os.ErrProcessDone are reported, and Wait
// always reaps the child.
func (up *uiProcess) stop(t *testing.T) {
	t.Helper()
	up.stopOnce.Do(func() {
		if err := up.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("signalling ui: %v", err)
		}
		select {
		case err := <-up.done:
			if err != nil {
				t.Errorf("ui exited with error: %v", err)
			}
		case <-time.After(10 * time.Second):
			if err := up.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Errorf("killing ui: %v", err)
			}
			select {
			case err := <-up.done:
				if err != nil {
					t.Errorf("ui exited with error after Kill: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("ui did not reap after Kill")
			}
			t.Fatal("ui did not shut down after SIGTERM")
		}
	})
}

// waitForURL blocks until the ui process prints its URL line on stdout,
// bounded by a deterministic timeout: readiness is signalled by the URL line
// (printed after the listener is bound, so connections queue in the kernel
// backlog) and there is no random sleep.
func waitForURL(t *testing.T, stdout io.Reader, stderr *bytes.Buffer) string {
	t.Helper()
	urls := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "UI available at ") {
				urls <- strings.TrimPrefix(line, "UI available at ")
				return
			}
		}
		close(urls)
	}()
	select {
	case url, ok := <-urls:
		if !ok {
			t.Fatalf("ui did not print its URL; stderr:\n%s", stderr.String())
		}
		return url
	case <-time.After(10 * time.Second):
		t.Fatalf("ui did not print its URL within 10s; stderr:\n%s", stderr.String())
	}
	return ""
}

func get(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func post(t *testing.T, client *http.Client, url, body string) *http.Response {
	t.Helper()
	resp, err := client.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}
func TestUIServerSetupAndRestart(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(t.TempDir(), "webui-store")
	client := &http.Client{Timeout: 5 * time.Second}

	up := startUI(t, home)

	// uninitialized through a real server process
	resp := get(t, client, up.url+"/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d", resp.StatusCode)
	}
	var st struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if st.State != "uninitialized" {
		t.Fatalf("state: got %q", st.State)
	}

	// setup through HTTP with a custom Store
	body := fmt.Sprintf(`{"store_path": %q}`, store)
	resp = post(t, client, up.url+"/api/v1/setup", body)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("setup: got %d, body %s", resp.StatusCode, b)
	}
	resp.Body.Close()
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("setup did not create the Store: %v", err)
	}

	resp = get(t, client, up.url+"/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status after setup: got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if st.State != "ready" {
		t.Fatalf("state after setup: got %q", st.State)
	}

	// SPA deep link, API isolation, and missing assets
	resp = get(t, client, up.url+"/skills/example-skill")
	html, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(html), `<div id="root">`) {
		t.Fatalf("SPA deep link: got %d", resp.StatusCode)
	}

	resp = get(t, client, up.url+"/api/v1/unknown")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown API route: got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("API 404 content type: got %q", ct)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	resp = get(t, client, up.url+"/assets/nope.js")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset: got %d", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// graceful shutdown
	up.stop(t)

	// restart: a fresh ui process reports ready from the persisted state
	up2 := startUI(t, home)
	defer up2.stop(t)
	resp = get(t, client, up2.url+"/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status after restart: got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if st.State != "ready" {
		t.Fatalf("state after restart: got %q", st.State)
	}
}
