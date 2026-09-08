// The synchronization action policy: which actions are legal for the
// current Sync Status, per decision 07. A conflict offers Keep Store and
// Accept Source; missing or invalid Source entries block retrieval
// actions. Kept free of components so the action bar stays the only
// component export of its file.
import type { Skill } from './skill-api'
import type { SyncStatus } from './sync-api'

export type SyncActionName = 'check' | 'sync' | 'keep_store' | 'accept_source' | 'rollback'

export type SyncDialogName = 'keep_store' | 'accept_source' | 'rollback'

const KEEP_STORE_STATUSES: SyncStatus[] = ['conflict', 'source_changed']

const ACCEPT_SOURCE_STATUSES: SyncStatus[] = [
  'conflict',
  'source_changed',
  'store_changed',
  'store_missing',
  'store_invalid',
]

export function syncActionsFor(skill: Skill): SyncActionName[] {
  if (!skill.binding) return ['check']
  const status = skill.sync_status
  const actions: SyncActionName[] = ['check']
  if (status !== 'conflict') actions.push('sync')
  if (KEEP_STORE_STATUSES.includes(status as SyncStatus)) actions.push('keep_store')
  if (ACCEPT_SOURCE_STATUSES.includes(status as SyncStatus)) actions.push('accept_source')
  // Rollback needs a previous snapshot to restore: never offer it for a
  // Skill whose last replacement could not record one.
  if (skill.has_previous_snapshot) actions.push('rollback')
  return actions
}
