package source

import (
	"strings"
	"testing"
)

const validPointer = "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\n"

func TestIsLFSPointer(t *testing.T) {
	good := func(name, content string) {
		t.Helper()
		if !isLFSPointer([]byte(content)) {
			t.Errorf("%s: %q must be a pointer", name, content)
		}
	}
	bad := func(name, content string) {
		t.Helper()
		if isLFSPointer([]byte(content)) {
			t.Errorf("%s: %q must not be a pointer", name, content)
		}
	}

	good("canonical", validPointer)
	good("no trailing newline", strings.TrimSuffix(validPointer, "\n"))
	good("size zero", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 0\n")
	good("uppercase oid hex", "version https://git-lfs.github.com/spec/v1\noid sha256:4D7A214614AB2935C943F9E0FF69D22EADBB97F32B8A4F3D1B1F2D0B1B2B3C4D\nsize 1\n")

	bad("empty", "")
	bad("ordinary text", "hello world\n")
	bad("one line", "version https://git-lfs.github.com/spec/v1\n")
	bad("two lines", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\n")
	bad("wrong version", "version https://git-lfs.github.com/spec/v2\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\n")
	bad("other oid algorithm", "version https://git-lfs.github.com/spec/v1\noid sha1:4d7a214614ab2935c943f9e0ff69d22eadbb97f32\nsize 12345\n")
	bad("short oid", "version https://git-lfs.github.com/spec/v1\noid sha256:abcd\nsize 12345\n")
	bad("non-hex oid", "version https://git-lfs.github.com/spec/v1\noid sha256:zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz\nsize 12345\n")
	bad("missing size", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\n")
	bad("negative size", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize -1\n")
	bad("non-numeric size", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize many\n")
	bad("overflowing size", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 99999999999999999999999999\n")
	bad("extra line", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\nextra\n")
	bad("extra trailing blank line", validPointer+"\n")
	bad("crlf endings", "version https://git-lfs.github.com/spec/v1\r\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\r\nsize 12345\r\n")
	bad("missing oid prefix", "version https://git-lfs.github.com/spec/v1\nsha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\nsize 12345\n")
	bad("missing size prefix", "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb97f32b8a4f3d1b1f2d0b1b2b3c4d\n12345\n")
}
