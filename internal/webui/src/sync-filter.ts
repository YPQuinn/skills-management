// Sync Status filters for the Skills index: counts and membership
// derived from fields already returned by GET /api/v1/skills.
import type { DictionaryKey } from './locale-dictionary'
import type { Skill } from './skill-api'
import type { SyncStatus } from './sync-api'

const STATUS_FILTERS = [
  'in_sync',
  'source_changed',
  'store_changed',
  'conflict',
  'unbound',
] as const satisfies readonly SyncStatus[]

export type SyncFilter = 'all' | 'stale' | (typeof STATUS_FILTERS)[number]

export const SYNC_FILTERS: SyncFilter[] = ['all', ...STATUS_FILTERS, 'stale']

export const syncFilterLabelKey: Record<SyncFilter, DictionaryKey> = {
  all: 'filterSyncAll',
  in_sync: 'filterSyncInSync',
  source_changed: 'filterSyncSourceChanged',
  store_changed: 'filterSyncStoreChanged',
  conflict: 'filterSyncConflict',
  unbound: 'filterSyncUnbound',
  stale: 'filterSyncStale',
}

export function skillMatchesSyncFilter(skill: Skill, filter: SyncFilter): boolean {
  if (filter === 'all') return true
  if (filter === 'stale') return skill.sync_stale
  return skill.sync_status === filter
}

export function countSyncFilters(skills: Skill[]): Record<SyncFilter, number> {
  const counts = Object.fromEntries(SYNC_FILTERS.map((filter) => [filter, 0])) as Record<SyncFilter, number>
  for (const skill of skills) {
    for (const filter of SYNC_FILTERS) {
      if (skillMatchesSyncFilter(skill, filter)) counts[filter] += 1
    }
  }
  return counts
}
