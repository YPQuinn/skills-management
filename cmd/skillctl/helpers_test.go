// Package main contains the separate-process end-to-end tests for skillctl.
// They build a temporary binary once per test run, isolate HOME per child
// process, use port 0 everywhere, and never sleep-poll: readiness is
// signalled by the URL line on stdout (printed after the listener is bound,
// so connections queue in the kernel backlog).
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	bin       string
	buildErr  error
)

// testBinary builds the skillctl executable once per test run into a
// temporary directory. The stale root ./skillctl is never executed.
func testBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "skillctl-e2e-*")
		if err != nil {
			buildErr = err
			return
		}
		bin = filepath.Join(dir, "skillctl")
		out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("building test binary: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return bin
}

// run executes the binary with an isolated HOME and returns combined output
// and the exit code.
func run(t *testing.T, home string, args ...string) (string, int) {
	t.Helper()
	stdout, stderr, code := runOut(t, home, args...)
	return stdout + stderr, code
}

// runOut executes the binary with an isolated HOME and returns stdout,
// stderr, and the exit code separately, so JSON-output contracts can be
// asserted without human stderr corrupting stdout.
func runOut(t *testing.T, home string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(testBinary(t), args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("running %v: %v", args, err)
	return "", "", -1
}
