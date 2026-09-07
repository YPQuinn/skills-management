package app

import (
	"skillctl/internal/distribution"
	"testing"
)

func createFixtureLink(t *testing.T, container, slug, raw string) (distribution.LinkProof, error) {
	t.Helper()
	slot, err := distribution.NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := distribution.CreateLink(container, slug, raw, slot, func(distribution.LinkProof) error { return nil })
	if cleanupErr := distribution.DiscardCreateStaging(container, slot, proof); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	return proof, err
}
