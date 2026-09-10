// Sync presentation policy: the ten-state relationship and the eight
// result values mapped to locale keys. Text always accompanies color, per
// decision 07. Kept free of components so the status renderers stay the
// only exports of their file.
import type { DictionaryKey } from './locale-dictionary'
import type { SyncActionResult, SyncStatus } from './sync-api'

export const syncStatusKey: Record<SyncStatus, DictionaryKey> = {
  unbound: 'statusSyncUnbound',
  unchecked: 'statusSyncUnchecked',
  in_sync: 'statusSyncInSync',
  source_changed: 'statusSyncSourceChanged',
  store_changed: 'statusSyncStoreChanged',
  conflict: 'statusSyncConflict',
  source_missing: 'statusSyncSourceMissing',
  source_invalid: 'statusSyncSourceInvalid',
  store_missing: 'statusSyncStoreMissing',
  store_invalid: 'statusSyncStoreInvalid',
}

export const syncResultKey: Record<SyncActionResult, DictionaryKey> = {
  no_op: 'resultNoOp',
  updated: 'resultUpdated',
  kept_store: 'resultKeptStore',
  accepted_source: 'resultAcceptedSource',
  skipped: 'resultSkipped',
  blocked: 'resultBlocked',
  failed: 'resultFailed',
  rolled_back: 'resultRolledBack',
}

export const syncActionKey: Record<string, DictionaryKey> = {
  sync: 'actionSync',
  keep_store: 'actionKeepStore',
  accept_source: 'actionAcceptSource',
  rollback: 'actionRollback',
}

export const syncConclusionKey: Partial<Record<SyncStatus, DictionaryKey>> = {
  in_sync: 'syncConclusionInSync',
  source_changed: 'syncConclusionSourceChanged',
  store_changed: 'syncConclusionStoreChanged',
  conflict: 'syncConclusionConflict',
  unchecked: 'syncConclusionUnchecked',
  source_missing: 'syncConclusionSourceMissing',
  source_invalid: 'syncConclusionSourceInvalid',
  store_missing: 'syncConclusionStoreMissing',
  store_invalid: 'syncConclusionStoreInvalid',
}

export function isSyncStatus(value: string): value is SyncStatus {
  return Object.prototype.hasOwnProperty.call(syncStatusKey, value)
}

export function isSyncActionResult(value: string): value is SyncActionResult {
  return Object.prototype.hasOwnProperty.call(syncResultKey, value)
}

const successfulSyncResults: Record<SyncActionResult, boolean> = {
  no_op: false,
  updated: true,
  kept_store: true,
  accepted_source: true,
  skipped: false,
  blocked: false,
  failed: false,
  rolled_back: true,
}

export function isSuccessfulSyncResult(value: string): boolean {
  return isSyncActionResult(value) && successfulSyncResults[value]
}

// Distinct from the negation of the above: a `no_op` did not succeed at
// anything, but it also asks nothing of the reader. These are the results
// worth naming a Skill over.
const attentionSyncResults: Record<SyncActionResult, boolean> = {
  no_op: false,
  updated: false,
  kept_store: false,
  accepted_source: false,
  skipped: true,
  blocked: true,
  failed: true,
  rolled_back: false,
}

export function syncResultNeedsAttention(value: string): boolean {
  return !isSyncActionResult(value) || attentionSyncResults[value]
}
