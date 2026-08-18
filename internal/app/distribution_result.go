package app

import (
	"time"

	"skillctl/internal/distribution"
)

// DistributionResult is the complete outcome of one reconciliation: the
// per-item results, the summary tallies, and the Target-level outcome
// (decision 06).
type DistributionResult struct {
	TargetID  int64                    `json:"target_id"`
	DryRun    bool                     `json:"dry_run"`
	Outcome   string                   `json:"outcome"`
	Error     string                   `json:"error,omitempty"`
	Inspected *time.Time               `json:"inspected_at"`
	Stale     bool                     `json:"stale"`
	Items     []DistributionItemResult `json:"items"`
	Summary   DistributionSummary      `json:"summary"`
}

// DistributionItemResult is one item's reconciliation result: the observed
// facts, the planned action, and the outcome. Dry-run items carry the
// predicted outcome as their result.
type DistributionItemResult struct {
	SkillID  int64  `json:"skill_id"`
	Slug     string `json:"slug"`
	Desired  string `json:"desired"`
	Observed string `json:"observed"`
	Action   string `json:"action"`
	Result   string `json:"result"`
	Error    string `json:"error,omitempty"`
}

// DistributionSummary tallies the per-item results of one reconciliation.
type DistributionSummary struct {
	Total           int `json:"total"`
	NoOp            int `json:"no_op"`
	Created         int `json:"created"`
	Removed         int `json:"removed"`
	Adopted         int `json:"adopted"`
	BlockedConflict int `json:"blocked_conflict"`
	BlockedBroken   int `json:"blocked_broken"`
	OwnershipLost   int `json:"ownership_lost"`
	Failed          int `json:"failed"`
}

func (s *DistributionSummary) add(result string) {
	switch result {
	case distribution.OutcomeNoOp:
		s.NoOp++
	case distribution.OutcomeCreated:
		s.Created++
	case distribution.OutcomeRemoved:
		s.Removed++
	case distribution.OutcomeAdopted:
		s.Adopted++
	case distribution.OutcomeBlockedConflict:
		s.BlockedConflict++
	case distribution.OutcomeBlockedBroken:
		s.BlockedBroken++
	case distribution.OutcomeOwnershipLost:
		s.OwnershipLost++
	case distribution.OutcomeFailed:
		s.Failed++
	}
}
