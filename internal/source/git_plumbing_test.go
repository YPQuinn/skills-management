package source

import (
	"strings"
	"testing"
)

// TestParseBatchOutputHandlesNewlinesInContent pins the byte-wise batch
// parser: blob content may contain newlines, and a record's size field is
// authoritative, so line-based parsing would corrupt the stream.
func TestParseBatchOutputHandlesNewlinesInContent(t *testing.T) {
	out := "0000000000000000000000000000000000000001 blob 11\nline1\nline2\n" +
		"0000000000000000000000000000000000000002 blob 3\nabc\n"
	blobs, err := parseBatchOutput([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(blobs["0000000000000000000000000000000000000001"]); got != "line1\nline2" {
		t.Fatalf("newline content: got %q", got)
	}
	if got := string(blobs["0000000000000000000000000000000000000002"]); got != "abc" {
		t.Fatalf("plain content: got %q", got)
	}
}

// TestParseBatchOutputMissingObject fails the whole batch when git reports
// a missing object, so a partial observation can never carry a digest
// computed from incomplete content.
func TestParseBatchOutputMissingObject(t *testing.T) {
	out := "0000000000000000000000000000000000000001 missing\n"
	if _, err := parseBatchOutput([]byte(out)); err == nil {
		t.Fatal("missing object: want error")
	}
	_, second := parseBatchOutput([]byte(out))
	if !strings.Contains(second.Error(), "missing") {
		t.Fatalf("missing-object error must name the failure: %v", second)
	}
}

// TestParseBatchOutputTruncated rejects a truncated record instead of
// silently returning partial content.
func TestParseBatchOutputTruncated(t *testing.T) {
	out := "0000000000000000000000000000000000000001 blob 10\nshort\n"
	if _, err := parseBatchOutput([]byte(out)); err == nil {
		t.Fatal("truncated content: want error")
	}
}

// TestParseBatchOutputTruncatedHeader rejects a stream that ends inside a
// record header instead of silently dropping the partial record.
func TestParseBatchOutputTruncatedHeader(t *testing.T) {
	out := "0000000000000000000000000000000000000001 blob 3"
	if _, err := parseBatchOutput([]byte(out)); err == nil {
		t.Fatal("truncated header: want error")
	}
}

// TestParseBatchOutputMalformedSize rejects sizes that are not exact
// non-negative integers, so a garbage size can never silently shift the
// content stream.
func TestParseBatchOutputMalformedSize(t *testing.T) {
	for _, out := range []string{
		"0000000000000000000000000000000000000001 blob 3abc\nabc\n",
		"0000000000000000000000000000000000000001 blob -1\nabc\n",
	} {
		if _, err := parseBatchOutput([]byte(out)); err == nil {
			t.Fatalf("malformed size %q: want error", out)
		}
	}
}
