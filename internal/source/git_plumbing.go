package source

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// parseTreeRecords parses `git ls-tree -z` output. Records are NUL
// separated and paths are never quoted, so paths may contain tabs,
// newlines, and any Unicode regardless of the repository's quoting
// configuration. The path is everything after the first tab; the metadata
// before it is exactly mode, type, and object id.
func parseTreeRecords(out string) ([]treeLine, error) {
	var rows []treeLine
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			return nil, fmt.Errorf("malformed ls-tree record")
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed ls-tree metadata %q", fields)
		}
		rows = append(rows, treeLine{mode: fields[0], typ: fields[1], oid: fields[2], path: path})
	}
	return rows, nil
}

// fetchTreeBlobs reads every blob referenced by the listed rows (regular
// files, symlinks, and SKILL.md markers) in one cat-file --batch call,
// returning a map keyed by object id. Missing blobs are fetched lazily by
// git from the promisor remote when the cache is a partial clone; the call
// carries the same ambient GitHub credential helper as every other network
// git invocation, so a private promisor fetch authenticates like the clone
// that created the cache. An object that is still missing fails the whole
// observation rather than producing a partial digest.
func fetchTreeBlobs(ctx context.Context, cache string, all []treeLine, loc Locator) (map[string][]byte, error) {
	oids := make([]string, 0, len(all))
	seen := map[string]bool{}
	for _, l := range all {
		if l.mode != "100644" && l.mode != "100755" && l.mode != "120000" {
			continue
		}
		if seen[l.oid] {
			continue
		}
		seen[l.oid] = true
		oids = append(oids, l.oid)
	}
	if len(oids) == 0 {
		return map[string][]byte{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	args := append(gitAuthArgs(loc), "-C", cache, "cat-file", "--batch")
	cmd := exec.CommandContext(ctx, gitBin(), args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var in strings.Builder
	for _, oid := range oids {
		in.WriteString(oid)
		in.WriteByte('\n')
	}
	cmd.Stdin = strings.NewReader(in.String())
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git cat-file --batch: %s", msg)
	}
	return parseBatchOutput(out.Bytes())
}

// parseBatchOutput parses `git cat-file --batch` responses: one record per
// requested object as "<oid> <type> <size>\n<content>\n". Content may
// contain newlines, so the stream is parsed byte-wise, never line-wise, and
// any truncation is an error rather than silently partial content.
func parseBatchOutput(out []byte) (map[string][]byte, error) {
	blobs := map[string][]byte{}
	buf := bytes.NewBuffer(out)
	for {
		header, err := buf.ReadBytes('\n')
		if err == io.EOF && len(header) == 0 {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("truncated cat-file --batch header")
		}
		fields := strings.Fields(string(header))
		if len(fields) == 2 && fields[1] == "missing" {
			return nil, fmt.Errorf("object %s is missing from the repository", fields[0])
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed cat-file --batch header %q", strings.TrimSpace(string(header)))
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("malformed cat-file --batch size %q", fields[2])
		}
		content := make([]byte, size)
		if _, err := io.ReadFull(buf, content); err != nil {
			return nil, fmt.Errorf("truncated cat-file --batch content")
		}
		if _, err := buf.ReadByte(); err != nil { // trailing newline
			return nil, fmt.Errorf("truncated cat-file --batch record")
		}
		blobs[fields[0]] = content
	}
	return blobs, nil
}

// isDotGitPath reports whether any path segment is ".git". Git refuses to
// track such paths, so this is a defensive filter mirroring the Local
// observer's exclusion of .git metadata directories.
func isDotGitPath(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".git" {
			return true
		}
	}
	return false
}
