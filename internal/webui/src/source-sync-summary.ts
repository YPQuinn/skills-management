// The bound-Skill Sync Status summary shown above the Source Inventory,
// derived from fields GET /api/v1/skills already returns.
import type { Skill } from './skill-api'

// Statuses needing a human decision come first, then the ones a batch
// Synchronization can act on, then the quiet ones.
const STATUS_ORDER = [
  'conflict',
  'store_missing',
  'store_invalid',
  'source_missing',
  'source_invalid',
  'store_changed',
  'source_changed',
  'unchecked',
  'in_sync',
  'unbound',
]

export interface StatusCount {
  status: string
  count: number
}

export interface BoundSyncSummary {
  total: number
  byStatus: StatusCount[]
  conflicts: number
  lastEvaluatedAt?: string
}

// summarizeBoundSync describes the last observed relationship of the Skills
// bound to one Source. A Source rescan does not re-evaluate these, so the
// reported time is the oldest evaluation among them: the summary is only as
// fresh as its stalest member, and one never-evaluated Skill leaves it
// undefined rather than borrowing a sibling's timestamp.
export function summarizeBoundSync(skills: Skill[]): BoundSyncSummary {
  const counts = new Map<string, number>()
  let conflicts = 0
  let oldest: number | undefined
  let anyUnevaluated = false

  for (const skill of skills) {
    const status = skill.sync_status || 'unchecked'
    counts.set(status, (counts.get(status) ?? 0) + 1)
    if (status === 'conflict') conflicts += 1

    const checked = skill.sync_checked_at ? Date.parse(skill.sync_checked_at) : NaN
    if (Number.isNaN(checked)) anyUnevaluated = true
    else if (oldest === undefined || checked < oldest) oldest = checked
  }

  const byStatus = [...counts]
    .map(([status, count]) => ({ status, count }))
    .sort((a, b) => rank(a.status) - rank(b.status))

  return {
    total: skills.length,
    byStatus,
    conflicts,
    lastEvaluatedAt:
      anyUnevaluated || oldest === undefined ? undefined : new Date(oldest).toISOString(),
  }
}

function rank(status: string): number {
  const index = STATUS_ORDER.indexOf(status)
  return index === -1 ? STATUS_ORDER.length : index
}
