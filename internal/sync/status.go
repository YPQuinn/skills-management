// Package sync owns the pure synchronization domain (decision 05): the
// ten-state three-way content relationship, the synchronization outcome
// vocabulary, and the three-way path differences. It never executes SQL and
// never touches the Skill Store; the application boundary supplies observed
// facts and persists the results.
package sync

// Status is one value of the observed Sync Status relationship between a
// Skill's Store content, its Synchronization Baseline, and its bound Source
// entry. These exact values are the stable contract shared by CLI, REST,
// and WebUI.
type Status string

const (
	StatusUnbound       Status = "unbound"
	StatusUnchecked     Status = "unchecked"
	StatusInSync        Status = "in_sync"
	StatusSourceChanged Status = "source_changed"
	StatusStoreChanged  Status = "store_changed"
	StatusConflict      Status = "conflict"
	StatusSourceMissing Status = "source_missing"
	StatusSourceInvalid Status = "source_invalid"
	StatusStoreMissing  Status = "store_missing"
	StatusStoreInvalid  Status = "store_invalid"
)

// Action names the latest synchronization operation a Skill recorded.
const (
	ActionSync         = "sync"
	ActionKeepStore    = "keep_store"
	ActionAcceptSource = "accept_source"
	ActionRollback     = "rollback"
)

// Result is the stable outcome of the latest synchronization action.
const (
	ResultNoOp           = "no_op"
	ResultUpdated        = "updated"
	ResultKeptStore      = "kept_store"
	ResultAcceptedSource = "accepted_source"
	ResultSkipped        = "skipped"
	ResultBlocked        = "blocked"
	ResultFailed         = "failed"
	ResultRolledBack     = "rolled_back"
)

// StatusInput carries the observed facts one three-way evaluation needs.
// Source facts come from one coherent observation; Store facts from the
// live Skill tree; the Baseline digest from the persisted acceptance.
type StatusInput struct {
	Bound         bool
	SourceAvail   bool
	SourceMissing bool
	SourceInvalid bool
	SourceDigest  string
	StoreMissing  bool
	StoreInvalid  bool
	StoreDigest   string
	Baseline      string
}

// Evaluate computes the content relationship from the observed facts.
// When the Source is unavailable the previous relationship is retained and
// marked stale (decision 05): an unavailable Source is never rewritten into
// a generic unavailable state. An unbound Skill is always unbound, fresh.
// Ordering of the remaining states follows the decision narrative: entry
// absence, entry invalidity, Store absence, Store invalidity, then the
// three-way digest comparison, where Source equals Store is in_sync even
// when the Baseline lags (independent convergence).
func Evaluate(prev Status, in StatusInput) (Status, bool) {
	if !in.SourceAvail {
		if prev == "" {
			return StatusUnchecked, true
		}
		return prev, true
	}
	if !in.Bound {
		return StatusUnbound, false
	}
	switch {
	case in.SourceMissing:
		return StatusSourceMissing, false
	case in.SourceInvalid:
		return StatusSourceInvalid, false
	case in.StoreMissing:
		return StatusStoreMissing, false
	case in.StoreInvalid:
		return StatusStoreInvalid, false
	}
	switch {
	case in.SourceDigest == in.Baseline && in.StoreDigest == in.Baseline:
		return StatusInSync, false
	case in.SourceDigest == in.StoreDigest:
		return StatusInSync, false
	case in.SourceDigest != in.Baseline && in.StoreDigest == in.Baseline:
		return StatusSourceChanged, false
	case in.SourceDigest == in.Baseline && in.StoreDigest != in.Baseline:
		return StatusStoreChanged, false
	default:
		return StatusConflict, false
	}
}

// RecheckStatus recomputes the relationship from persisted digests after a
// content-changing action, without a fresh observation: the Binding digest
// is the last accepted Source content. The caller sets the stale flag from
// the Source row's persisted availability; the digest comparison itself
// always runs so an action's immediate effect is visible even when the
// Source is currently unreachable.
func RecheckStatus(in StatusInput) Status {
	in.SourceAvail = true
	status, _ := Evaluate(StatusUnchecked, in)
	return status
}
