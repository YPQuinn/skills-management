package skillstore

import (
	"context"
	"os"
	"testing"
)

// TestStageSyncsOperationDirectoryEntryBeforeReturn pins the durability
// ordering required before Install's first live mutation: the operation
// directory entry is durably synced in its staging parent before Stage
// returns. On power loss after a synced live rename but before the install
// proof is written, recovery must still find the operation directory (or
// fail closed on its surviving staging), never classify the operation as
// evidence-free and clear the row while unbound live content survives. The
// sync seam fires exactly once, and only after the entry exists and refers
// to the created object.
func TestStageSyncsOperationDirectoryEntryBeforeReturn(t *testing.T) {
	s := newStore(t)
	m := buildMaterialized(t, map[string]string{"SKILL.md": "content"})
	fired := 0
	testAfterOpDirEntrySync = func(staging *os.Root, opDirID fileID) {
		fired++
		if err := nameRefersTo(staging, "1", opDirID); err != nil {
			t.Fatalf("the operation directory entry must be present and bound when the sync seam fires: %v", err)
		}
	}
	defer func() { testAfterOpDirEntrySync = nil }()

	if _, err := s.Stage(context.Background(), 1, m.dir, m.digest, false); err != nil {
		t.Fatal(err)
	}
	if fired != 1 {
		t.Fatalf("the operation-directory sync seam must fire exactly once, fired %d", fired)
	}
}
