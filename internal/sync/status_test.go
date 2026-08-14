package sync

import "testing"

// TestEvaluateStatusMatrix locks the decision-05 ten-state relationship
// matrix plus the staleness rules.
func TestEvaluateStatusMatrix(t *testing.T) {
	d1 := "d1"
	d2 := "d2"
	d3 := "d3"
	tests := []struct {
		name  string
		prev  Status
		in    StatusInput
		want  Status
		stale bool
	}{
		{name: "unbound", in: StatusInput{Bound: false, SourceAvail: true}, want: StatusUnbound},
		{name: "unavailable retains and marks stale", prev: StatusSourceChanged,
			in: StatusInput{Bound: true, SourceAvail: false}, want: StatusSourceChanged, stale: true},
		{name: "unavailable without previous is unchecked", prev: "",
			in: StatusInput{Bound: true, SourceAvail: false}, want: StatusUnchecked, stale: true},
		{name: "in sync", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d1, StoreDigest: d1, Baseline: d1}, want: StatusInSync},
		{name: "source changed", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d2, StoreDigest: d1, Baseline: d1}, want: StatusSourceChanged},
		{name: "store changed", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d1, StoreDigest: d2, Baseline: d1}, want: StatusStoreChanged},
		{name: "conflict", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d2, StoreDigest: d3, Baseline: d1}, want: StatusConflict},
		{name: "converged is in sync despite stale baseline", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d2, StoreDigest: d2, Baseline: d1}, want: StatusInSync},
		{name: "source missing", in: StatusInput{Bound: true, SourceAvail: true,
			SourceMissing: true, StoreDigest: d1, Baseline: d1}, want: StatusSourceMissing},
		{name: "source invalid", in: StatusInput{Bound: true, SourceAvail: true,
			SourceInvalid: true, StoreDigest: d1, Baseline: d1}, want: StatusSourceInvalid},
		{name: "store missing", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d1, StoreMissing: true, Baseline: d1}, want: StatusStoreMissing},
		{name: "store invalid", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d1, StoreInvalid: true, Baseline: d1}, want: StatusStoreInvalid},
		{name: "unavailable never rewrites to missing", prev: StatusConflict,
			in: StatusInput{Bound: true, SourceAvail: false, SourceMissing: true}, want: StatusConflict, stale: true},
		{name: "missing before invalid", in: StatusInput{Bound: true, SourceAvail: true,
			SourceMissing: true, SourceInvalid: true, StoreDigest: d1, Baseline: d1}, want: StatusSourceMissing},
		{name: "source problem before store problem", in: StatusInput{Bound: true, SourceAvail: true,
			SourceMissing: true, StoreMissing: true}, want: StatusSourceMissing},
		{name: "store problem before digest comparison", in: StatusInput{Bound: true, SourceAvail: true,
			SourceDigest: d2, StoreMissing: true, Baseline: d1}, want: StatusStoreMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, stale := Evaluate(tt.prev, tt.in)
			if got != tt.want || stale != tt.stale {
				t.Fatalf("Evaluate(%q) = %q, %v; want %q, %v", tt.prev, got, stale, tt.want, tt.stale)
			}
		})
	}
}

// TestRecheckStatus forces the digest comparison even while the Source is
// unavailable, so an action's immediate effect is always visible.
func TestRecheckStatus(t *testing.T) {
	in := StatusInput{Bound: true, SourceAvail: false,
		SourceDigest: "a", StoreDigest: "b", Baseline: "a"}
	if got := RecheckStatus(in); got != StatusStoreChanged {
		t.Fatalf("RecheckStatus = %q, want store_changed", got)
	}
}
