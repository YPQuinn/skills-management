package source

import (
	"fmt"
	"os"
)

// openDestinationRoot creates and anchors a caller-owned output directory,
// rejecting a symlink or a swap between validation and open. Once returned,
// all writes remain beneath the opened directory even if its path is moved.
func openDestinationRoot(dst string) (*os.Root, error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	expected, err := os.Lstat(dst)
	if err != nil {
		return nil, err
	}
	if !expected.IsDir() {
		return nil, fmt.Errorf("materialization destination is not a directory")
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	if !os.SameFile(expected, opened) {
		root.Close()
		return nil, fmt.Errorf("materialization destination changed while it was being opened")
	}
	return root, nil
}
