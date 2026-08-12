package source

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// materializeBuffer is the size below which a regular file is read fully
// into memory so its bytes can be checked for a Git LFS pointer; larger
// files are streamed. A canonical LFS pointer is at most a few hundred
// bytes.
const materializeBuffer = 64 << 10

// lfsPointerError describes a file that is only an LFS pointer, whose real
// content was never fetched into the Source.
func lfsPointerError(path string) error {
	return fmt.Errorf("%s is a Git LFS pointer (unmaterialized content); run \"git lfs fetch\" and \"git lfs checkout\" in the Source and import again", path)
}

// lfsVersionLine is the exact version header of a canonical Git LFS v1
// pointer.
const lfsVersionLine = "version https://git-lfs.github.com/spec/v1"

// isLFSPointer reports whether b is a canonical Git LFS v1 pointer: exactly
// the version line, an oid sha256 line carrying 64 hex digits, and a
// non-negative size line, with at most one trailing newline. No other
// shape — other versions, other oid algorithms, extra lines, CRLF endings —
// is treated as a pointer, so ordinary text files never match.
func isLFSPointer(b []byte) bool {
	text := strings.TrimSuffix(string(b), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) != 3 {
		return false
	}
	if lines[0] != lfsVersionLine {
		return false
	}
	oid, ok := strings.CutPrefix(lines[1], "oid sha256:")
	if !ok || len(oid) != 64 {
		return false
	}
	if _, err := hex.DecodeString(oid); err != nil {
		return false
	}
	size, ok := strings.CutPrefix(lines[2], "size ")
	if !ok {
		return false
	}
	if _, err := strconv.ParseUint(size, 10, 64); err != nil {
		return false
	}
	return true
}
