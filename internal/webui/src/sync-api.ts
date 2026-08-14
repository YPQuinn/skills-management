// Sync REST/JSON boundary: resource shapes returned by the Skill
// synchronization routes (/api/v1/skills/{id}/check, /diff, /sync,
// /keep-store, /accept-source, /rollback) and the Source batch route
// /api/v1/sources/{id}/sync. The REST adapter owns snake_case field
// names, so these types mirror the Go DTOs.
import { type DictionaryKey } from './locale-dictionary'
import type { Skill } from './skill-api'
import { responseError } from './skill-api'

export type SyncStatus =
  | 'unbound'
  | 'unchecked'
  | 'in_sync'
  | 'source_changed'
  | 'store_changed'
  | 'conflict'
  | 'source_missing'
  | 'source_invalid'
  | 'store_missing'
  | 'store_invalid'

export type SyncActionResult =
  | 'no_op'
  | 'updated'
  | 'kept_store'
  | 'accepted_source'
  | 'skipped'
  | 'blocked'
  | 'failed'
  | 'rolled_back'

export interface LastSync {
  action: string
  result: string
  started_at?: string
  completed_at?: string
  before_digest?: string
  after_digest?: string
  revision?: string
  error?: string
}

export interface SyncItemResult {
  skill_id: number
  slug: string
  status: string
  stale: boolean
  action: string
  result: string
  code?: string
  message?: string
  before_digest?: string
  after_digest?: string
  revision?: string
}

export interface SyncSummary {
  total: number
  no_op: number
  updated: number
  kept_store: number
  accepted_source: number
  skipped: number
  blocked: number
  failed: number
  rolled_back: number
}

export interface SyncBatchResult {
  items: SyncItemResult[]
  summary: SyncSummary
}

export interface DiffNode {
  kind: string
  exec?: boolean
  size?: number
  digest?: string
}

export interface DiffText {
  unified: string
}

export interface DiffEntry {
  path: string
  changes: string[]
  from?: DiffNode
  to?: DiffNode
  text?: DiffText
}

export interface DiffComparison {
  from: string
  to: string
  entries: DiffEntry[]
}

export interface DiffResult {
  skill_id: number
  slug: string
  source_digest: string
  store_digest: string
  baseline_digest: string
  comparisons: DiffComparison[]
}

function postSyncAction(
  path: string,
  fallbackKey: DictionaryKey,
  signal?: AbortSignal,
): Promise<SyncItemResult> {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, fallbackKey, { status: res.status })
    return (await res.json()) as SyncItemResult
  })
}

export function checkSkillSync(id: number, signal?: AbortSignal): Promise<Skill> {
  return fetch(`/api/v1/skills/${id}/check`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCheckingSkillSyncFailed', { status: res.status })
    return (await res.json()) as Skill
  })
}

export function syncSkill(id: number, signal?: AbortSignal): Promise<SyncItemResult> {
  return postSyncAction(`/api/v1/skills/${id}/sync`, 'errSyncingSkillFailed', signal)
}

export function keepStore(id: number, signal?: AbortSignal): Promise<SyncItemResult> {
  return postSyncAction(`/api/v1/skills/${id}/keep-store`, 'errKeepStoreFailed', signal)
}

export function acceptSource(id: number, signal?: AbortSignal): Promise<SyncItemResult> {
  return postSyncAction(`/api/v1/skills/${id}/accept-source`, 'errAcceptSourceFailed', signal)
}

export function rollbackSkill(id: number, signal?: AbortSignal): Promise<SyncItemResult> {
  return postSyncAction(`/api/v1/skills/${id}/rollback`, 'errRollbackFailed', signal)
}

export function fetchSkillDiff(id: number, path?: string, signal?: AbortSignal): Promise<DiffResult> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return fetch(`/api/v1/skills/${id}/diff${query}`, { signal }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errLoadingDiffFailed', { status: res.status })
    return (await res.json()) as DiffResult
  })
}

export function syncSourceSkills(sourceId: number, signal?: AbortSignal): Promise<SyncBatchResult> {
  return fetch(`/api/v1/sources/${sourceId}/sync`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errSyncingSourceFailed', { status: res.status })
    return (await res.json()) as SyncBatchResult
  })
}
