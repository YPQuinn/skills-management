package sync

import (
	"fmt"
	"strings"
)

// maxDiffD bounds the Myers edit-distance search. A pair of files whose
// shortest edit script exceeds this bound is rendered as a whole-file
// replacement instead, which is always bounded and correct.
const maxDiffD = 1000

// UnifiedText renders a unified diff of two text files (without the file
// header lines) with three context lines per hunk. Empty content is valid
// for either side (additions and deletions). A diff too large to compute
// within the bound falls back to one whole-file replacement hunk.
func UnifiedText(a, b string) string {
	al := splitLines(a)
	bl := splitLines(b)
	edits, ok := diffLines(al, bl)
	if !ok {
		edits = replaceAll(al, bl)
	}
	return renderUnified(al, bl, edits)
}

// splitLines splits text into lines, treating a final newline as an empty
// terminator line so "a\n" and "a" differ exactly like diff(1). Without a
// trailing newline every split element is real content, so the last line
// must be kept: "a\nb" is two lines, never one.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// editOp is one line-level edit: keep, insert (b), or delete (a).
type editOp struct {
	kind byte // ' ', '+', '-'
	line string
}

// replaceAll is the bounded fallback edit script: remove every old line,
// add every new line.
func replaceAll(a, b []string) []editOp {
	edits := make([]editOp, 0, len(a)+len(b))
	for _, l := range a {
		edits = append(edits, editOp{kind: '-', line: l})
	}
	for _, l := range b {
		edits = append(edits, editOp{kind: '+', line: l})
	}
	return edits
}

// diffLines runs the O(ND) Myers shortest-edit-script search and returns
// the resulting edit script, or ok=false when the distance exceeds the
// bound.
func diffLines(a, b []string) ([]editOp, bool) {
	n, m := len(a), len(b)
	if n+m == 0 {
		return nil, true
	}
	limit := n + m
	offset := limit
	v := make([]int, 2*limit+1)
	v[offset+1] = 0
	trace := make([][]int, 0, limit+1)
	for d := 0; d <= limit && d <= maxDiffD; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, offset, a, b, d, k), true
			}
		}
	}
	return nil, false
}

// backtrack reconstructs the edit script by walking the recorded V arrays
// in reverse from the reaching (d, k) state. trace[d] holds the V values
// the forward search consulted when it chose the moves of round d, so the
// move decision of round d reads from trace[d] and the next round reads
// from trace[d-1].
func backtrack(trace [][]int, offset int, a, b []string, d, k int) []editOp {
	edits := make([]editOp, 0, len(a)+len(b))
	x, y := len(a), len(b)
	for d > 0 {
		vPrev := trace[d]
		var prevK int
		switch {
		case k == -d || (k != d && vPrev[offset+k-1] < vPrev[offset+k+1]):
			prevK = k + 1
		default:
			prevK = k - 1
		}
		prevX := vPrev[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, editOp{kind: ' ', line: a[x]})
		}
		if x == prevX {
			y--
			edits = append(edits, editOp{kind: '+', line: b[y]})
		} else {
			x--
			edits = append(edits, editOp{kind: '-', line: a[x]})
		}
		k = prevK
		d--
	}
	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, editOp{kind: ' ', line: a[x]})
	}
	for y > 0 {
		y--
		edits = append(edits, editOp{kind: '+', line: b[y]})
	}
	for x > 0 {
		x--
		edits = append(edits, editOp{kind: '-', line: a[x]})
	}
	// The edits were built backwards; reverse into the forward order.
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
	return edits
}

// renderUnified groups an edit script into hunks with three context lines
// and renders the standard unified body. Change positions whose context
// windows touch or overlap merge into one hunk.
func renderUnified(a, b []string, edits []editOp) string {
	type hunkRange struct{ begin, end int }
	var windows []hunkRange
	for i, e := range edits {
		if e.kind == ' ' {
			continue
		}
		w := hunkRange{begin: i - 3, end: i + 3}
		if n := len(windows); n > 0 && w.begin <= windows[n-1].end {
			windows[n-1].end = w.end
		} else {
			windows = append(windows, w)
		}
	}
	if len(windows) == 0 {
		return ""
	}
	var out strings.Builder
	for _, w := range windows {
		if w.begin < 0 {
			w.begin = 0
		}
		if w.end > len(edits) {
			w.end = len(edits)
		}
		aStart, bStart := 0, 0
		for i := 0; i < w.begin; i++ {
			if edits[i].kind != '+' {
				aStart++
			}
			if edits[i].kind != '-' {
				bStart++
			}
		}
		aCount, bCount := 0, 0
		var body strings.Builder
		for i := w.begin; i < w.end; i++ {
			e := edits[i]
			switch e.kind {
			case ' ':
				aCount++
				bCount++
				body.WriteString(" ")
			case '-':
				aCount++
				body.WriteString("-")
			case '+':
				bCount++
				body.WriteString("+")
			}
			body.WriteString(e.line)
			body.WriteByte('\n')
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)
		out.WriteString(body.String())
	}
	return out.String()
}
