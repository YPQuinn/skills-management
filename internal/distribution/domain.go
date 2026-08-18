// Package distribution owns the pure Distribution domain (decision 06):
// the desired and observed presence vocabulary, the reconciliation plan,
// the per-item and Target outcome vocabulary, and the filesystem
// primitives that inspect and mutate registered Target entries. It never
// executes SQL; the application boundary supplies the observed facts and
// persists the results.
package distribution

// Desired state of one Skill at a Target.
const (
	DesiredPresent = "present"
	DesiredAbsent  = "absent"
)

// Observed state of one Skill entry at a Target. These exact values are
// the stable contract shared by CLI, REST, and WebUI.
const (
	ObservedLinked     = "linked"
	ObservedMissing    = "missing"
	ObservedConflict   = "conflict"
	ObservedBrokenLink = "broken_link"
)

// Per-item reconciliation outcomes (decision 06 closed list).
const (
	OutcomeNoOp            = "no_op"
	OutcomeCreated         = "created"
	OutcomeRemoved         = "removed"
	OutcomeAdopted         = "adopted"
	OutcomeBlockedConflict = "blocked_conflict"
	OutcomeBlockedBroken   = "blocked_broken"
	OutcomeOwnershipLost   = "ownership_lost"
	OutcomeFailed          = "failed"
)

// Target-level reconciliation outcomes (decision 06 closed list).
const (
	ResultSucceeded = "succeeded"
	ResultPartial   = "partial"
	ResultBlocked   = "blocked"
	ResultFailed    = "failed"
)

// PlanItem derives one relation's reconciliation action from its desired
// and observed state and whether the Store Skill it points at is usable.
// A desired Skill whose Store content is missing or invalid stays desired
// and blocked (decision 06): it can never be created into a broken link.
func PlanItem(desired, observed string, storeOK bool) string {
	if desired != DesiredPresent {
		switch observed {
		case ObservedLinked, ObservedBrokenLink:
			return OutcomeRemoved
		case ObservedConflict:
			return OutcomeOwnershipLost
		default:
			return OutcomeRemoved // clear the stale ownership record
		}
	}
	switch observed {
	case ObservedLinked:
		return OutcomeNoOp
	case ObservedConflict:
		return OutcomeBlockedConflict
	case ObservedBrokenLink:
		return OutcomeBlockedBroken
	case ObservedMissing:
		if storeOK {
			return OutcomeCreated
		}
		return OutcomeBlockedBroken
	default:
		return OutcomeFailed
	}
}

// TargetOutcome reduces a complete per-item result set to the Target-level
// outcome: succeeded when every entry is satisfied or safely reconciled,
// blocked when conflicts permit no required progress, partial when some
// entries progressed while others are blocked or failed, and failed when
// no progress happened and an operational failure occurred.
func TargetOutcome(results []string) string {
	progress, blocked, failed := false, false, false
	for _, r := range results {
		switch r {
		case OutcomeNoOp, OutcomeCreated, OutcomeRemoved, OutcomeAdopted, OutcomeOwnershipLost:
			progress = true
		case OutcomeBlockedConflict, OutcomeBlockedBroken:
			blocked = true
		case OutcomeFailed:
			failed = true
		}
	}
	switch {
	case failed && !progress:
		return ResultFailed
	case blocked && !progress:
		return ResultBlocked
	case blocked || failed:
		return ResultPartial
	default:
		return ResultSucceeded
	}
}
