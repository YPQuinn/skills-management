package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateIdentityDoesNotClaimPreSampleReplacement(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "target")
	raw := filepath.Join(base, "store", "demo")
	link := filepath.Join(container, "demo")
	testAfterCreateBeforeStat = func() {
		replaceSameRaw(t, link, raw)
	}
	t.Cleanup(func() { testAfterCreateBeforeStat = nil })
	proof, err := createTestLink(t, container, "demo", raw)
	if err != nil {
		if got, readErr := os.Readlink(link); readErr != nil || got != raw {
			t.Fatalf("replacement was not preserved: %q, %v", got, readErr)
		}
		return
	}
	if err := VerifyLink(container, "demo", proof); err == nil {
		t.Error("created proof incorrectly authorizes an externally replaced symlink")
	}
	result, err := RemoveManagedLink(container, "demo", proof, mustIsolation(t))
	if _, statErr := os.Lstat(link); os.IsNotExist(statErr) {
		t.Fatalf("external replacement deleted: remove result=%v, error=%v", result, err)
	}
}
