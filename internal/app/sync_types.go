package app

import (
	"time"

	"skillctl/internal/sync"
)

// CodeSyncFailed is the stable per-item failure code of a synchronization
// action whose Store transition could not complete.
const CodeSyncFailed = "sync_failed"

// SyncItemResult is the stable per-item outcome of one synchronization
// action (decision 05): the evaluated Sync Status, the attempted action,
// the result value, the before and after content digests, the observed
// Source revision, and an optional failure detail.
type SyncItemResult struct {
	SkillID      int64       `json:"skill_id"`
	Slug         string      `json:"slug"`
	Status       sync.Status `json:"status"`
	Stale        bool        `json:"stale"`
	Action       string      `json:"action"`
	Result       string      `json:"result"`
	ErrorCode    string      `json:"error_code,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
	BeforeDigest string      `json:"before_digest,omitempty"`
	AfterDigest  string      `json:"after_digest,omitempty"`
	Revision     string      `json:"revision,omitempty"`
}

// LastSync is the persisted latest synchronization outcome shown on a Skill
// view.
type LastSync struct {
	Action       string     `json:"action"`
	Result       string     `json:"result"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	BeforeDigest string     `json:"before_digest,omitempty"`
	AfterDigest  string     `json:"after_digest,omitempty"`
	Revision     string     `json:"revision,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// SyncSkillsSummary tallies the per-item results of one synchronization
// batch.
type SyncSkillsSummary struct {
	Total          int `json:"total"`
	NoOp           int `json:"no_op"`
	Updated        int `json:"updated"`
	KeptStore      int `json:"kept_store"`
	AcceptedSource int `json:"accepted_source"`
	Skipped        int `json:"skipped"`
	Blocked        int `json:"blocked"`
	Failed         int `json:"failed"`
	RolledBack     int `json:"rolled_back"`
}

// SyncSkillsResult is the complete outcome of one valid synchronization
// batch: every item result plus the summary. A batch that began processing
// returns item outcomes rather than a top-level error.
type SyncSkillsResult struct {
	Items   []SyncItemResult  `json:"items"`
	Summary SyncSkillsSummary `json:"summary"`
}

// SummarizeSyncItems tallies one batch's item outcomes.
func SummarizeSyncItems(items []SyncItemResult) SyncSkillsSummary {
	var s SyncSkillsSummary
	s.Total = len(items)
	for _, it := range items {
		switch it.Result {
		case sync.ResultNoOp:
			s.NoOp++
		case sync.ResultUpdated:
			s.Updated++
		case sync.ResultKeptStore:
			s.KeptStore++
		case sync.ResultAcceptedSource:
			s.AcceptedSource++
		case sync.ResultSkipped:
			s.Skipped++
		case sync.ResultBlocked:
			s.Blocked++
		case sync.ResultFailed:
			s.Failed++
		case sync.ResultRolledBack:
			s.RolledBack++
		}
	}
	return s
}
