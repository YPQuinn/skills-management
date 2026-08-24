package distribution

import (
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/target"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// physicalTemp returns a temp dir whose path is already prefix-resolved,
// matching how the application normalizes registered Target and Store
// paths (macOS /var is a symlink to /private/var).
func mustCreateLink(t *testing.T, container, slug, raw string) LinkProof {
	t.Helper()
	p, err := CreateLink(container, slug, raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func dummyProof(raw string) LinkProof {
	return LinkProof{Raw: raw, Dev: 1, Ino: 1, Mtime: 1}
}

func replaceSameRaw(t *testing.T, link, raw string) {
	t.Helper()
	sib := link + ".new"
	if err := os.Symlink(raw, sib); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sib, link); err != nil {
		t.Fatal(err)
	}
}

func physicalTemp(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestPlanItemMatrix(t *testing.T) {
	cases := []struct {
		desired, observed string
		storeOK           bool
		want              string
	}{
		{DesiredPresent, ObservedLinked, true, OutcomeNoOp},
		{DesiredPresent, ObservedMissing, true, OutcomeCreated},
		{DesiredPresent, ObservedMissing, false, OutcomeBlockedBroken},
		{DesiredPresent, ObservedConflict, true, OutcomeBlockedConflict},
		{DesiredPresent, ObservedConflict, false, OutcomeBlockedConflict},
		{DesiredPresent, ObservedBrokenLink, true, OutcomeBlockedBroken},
		{DesiredPresent, ObservedBrokenLink, false, OutcomeBlockedBroken},
		{DesiredAbsent, ObservedLinked, false, OutcomeRemoved},
		{DesiredAbsent, ObservedBrokenLink, false, OutcomeRemoved},
		{DesiredAbsent, ObservedMissing, false, OutcomeRemoved},
		{DesiredAbsent, ObservedConflict, false, OutcomeOwnershipLost},
	}
	for _, c := range cases {
		if got := PlanItem(c.desired, c.observed, c.storeOK); got != c.want {
			t.Errorf("PlanItem(%s, %s, %v) = %s, want %s", c.desired, c.observed, c.storeOK, got, c.want)
		}
	}
}

func TestTargetOutcome(t *testing.T) {
	cases := []struct {
		results []string
		want    string
	}{
		{nil, ResultSucceeded},
		{[]string{OutcomeNoOp}, ResultSucceeded},
		{[]string{OutcomeCreated, OutcomeRemoved, OutcomeNoOp}, ResultSucceeded},
		{[]string{OutcomeOwnershipLost}, ResultSucceeded},
		{[]string{OutcomeBlockedConflict}, ResultBlocked},
		{[]string{OutcomeBlockedConflict, OutcomeBlockedBroken}, ResultBlocked},
		{[]string{OutcomeFailed}, ResultFailed},
		{[]string{OutcomeCreated, OutcomeBlockedConflict}, ResultPartial},
		{[]string{OutcomeNoOp, OutcomeFailed}, ResultPartial},
		{[]string{OutcomeNoOp, OutcomeBlockedBroken}, ResultPartial},
		{[]string{OutcomeCreated, OutcomeFailed}, ResultPartial},
	}
	for _, c := range cases {
		if got := TargetOutcome(c.results); got != c.want {
			t.Errorf("TargetOutcome(%v) = %s, want %s", c.results, got, c.want)
		}
	}
}

// TestGateTargetRedirected proves a symlink introduced into a prefix after
// registration redirects the Target and fails the gate, even when the new
// location looks safe.
func TestGateTargetRedirected(t *testing.T) {
	base := physicalTemp(t)
	if err := ensureDir(filepath.Join(base, "other", "skills")); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(filepath.Join(base, "other"), alias); err != nil {
		t.Fatal(err)
	}
	// The registered path's prefix now goes through the symlink.
	gate := GateTarget(filepath.Join(alias, "skills"), filepath.Join(base, "store"), target.ResolveOptions{})
	if gate.State != StateRedirected {
		t.Fatalf("gate state: %+v", gate)
	}
}

func TestGateTargetStates(t *testing.T) {
	base := physicalTemp(t)
	store := filepath.Join(base, "store")
	if err := ensureDir(store); err != nil {
		t.Fatal(err)
	}
	opts := target.ResolveOptions{}

	// missing container is the normal missing state
	missing := filepath.Join(base, "skills")
	if gate := GateTarget(missing, store, opts); gate.State != StateMissing {
		t.Fatalf("missing gate: %+v", gate)
	}
	// existing directory is ok
	if err := ensureDir(missing); err != nil {
		t.Fatal(err)
	}
	if gate := GateTarget(missing, store, opts); gate.State != StateOK {
		t.Fatalf("ok gate: %+v", gate)
	}
	// an existing non-directory is invalid
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if gate := GateTarget(file, store, opts); gate.State != StateInvalid {
		t.Fatalf("file gate: %+v", gate)
	}
	// the filesystem root is invalid
	if gate := GateTarget("/", store, opts); gate.State != StateInvalid {
		t.Fatalf("root gate: %+v", gate)
	}
	// a Target inside the Store is invalid
	inside := filepath.Join(store, "skills")
	if gate := GateTarget(inside, store, opts); gate.State != StateInvalid {
		t.Fatalf("overlap gate: %+v", gate)
	}
	// a Target that is an ancestor of the Store is invalid
	if gate := GateTarget(base, store, opts); gate.State != StateInvalid {
		t.Fatalf("ancestor gate: %+v", gate)
	}
}
