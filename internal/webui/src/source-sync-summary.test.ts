import { describe, it, expect } from 'vitest'
import { summarizeBoundSync } from './source-sync-summary'
import { skillAlpha } from './skill-fixtures'
import type { Skill } from './skill-api'

function bound(status: string, checkedAt?: string): Skill {
  return { ...skillAlpha, sync_status: status, sync_checked_at: checkedAt }
}

describe('summarizeBoundSync', () => {
  it('counts each observed status and puts the ones needing a decision first', () => {
    const summary = summarizeBoundSync([
      bound('in_sync', '2026-08-11T10:00:00Z'),
      bound('source_changed', '2026-08-11T10:00:00Z'),
      bound('conflict', '2026-08-11T10:00:00Z'),
      bound('in_sync', '2026-08-11T10:00:00Z'),
    ])

    expect(summary.total).toBe(4)
    expect(summary.conflicts).toBe(1)
    expect(summary.byStatus).toEqual([
      { status: 'conflict', count: 1 },
      { status: 'source_changed', count: 1 },
      { status: 'in_sync', count: 2 },
    ])
  })

  it('reports the oldest evaluation so the summary is never fresher than its stalest member', () => {
    const summary = summarizeBoundSync([
      bound('in_sync', '2026-08-11T12:00:00Z'),
      bound('in_sync', '2026-08-11T09:30:00Z'),
      bound('in_sync', '2026-08-11T11:00:00Z'),
    ])
    expect(summary.lastEvaluatedAt).toBe('2026-08-11T09:30:00.000Z')
  })

  it('reports no evaluation time when any bound Skill was never evaluated', () => {
    const summary = summarizeBoundSync([
      bound('in_sync', '2026-08-11T12:00:00Z'),
      bound('unchecked', undefined),
    ])
    expect(summary.lastEvaluatedAt).toBeUndefined()
    expect(summary.byStatus).toEqual([
      { status: 'unchecked', count: 1 },
      { status: 'in_sync', count: 1 },
    ])
  })

  it('treats an empty status as unchecked and keeps unknown statuses last', () => {
    const summary = summarizeBoundSync([
      bound('', '2026-08-11T10:00:00Z'),
      bound('something_new', '2026-08-11T10:00:00Z'),
      bound('conflict', '2026-08-11T10:00:00Z'),
    ])
    expect(summary.byStatus).toEqual([
      { status: 'conflict', count: 1 },
      { status: 'unchecked', count: 1 },
      { status: 'something_new', count: 1 },
    ])
  })

  it('summarizes an empty bound set without inventing a timestamp', () => {
    expect(summarizeBoundSync([])).toEqual({
      total: 0,
      byStatus: [],
      conflicts: 0,
      lastEvaluatedAt: undefined,
    })
  })
})
