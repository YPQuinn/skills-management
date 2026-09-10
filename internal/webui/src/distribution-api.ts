// Distribution REST/JSON boundary: the Target distribution routes
// (/api/v1/targets/{id}/inspect, /distribute, /adopt). The REST adapter
// owns snake_case field names, so these types mirror the Go DTOs.
import { type DictionaryKey } from './locale-dictionary'
import { responseError } from './skill-api'

export interface DistributionItemView {
  skill_id: number
  slug: string
  desired: 'present' | 'absent'
  observed: 'linked' | 'missing' | 'conflict' | 'broken_link'
  managed: boolean
  adoptable: boolean
  node_kind?: string
  raw_target?: string
  resolved_target?: string
  expected_path?: string
  stale: boolean
  last_result?: string
  last_error?: string
}

export interface DistributionStatus {
  target_id: number
  name: string
  path: string
  state: string
  state_error?: string
  inspected_at?: string
  stale: boolean
  inspection_error?: string
  items: DistributionItemView[]
  last_result: string
  last_started_at?: string
  last_completed_at?: string
  last_error?: string
}

export interface DistributionItemResult {
  skill_id: number
  slug: string
  desired: string
  observed: string
  action: string
  result: string
  error?: string
}

export interface DistributionSummary {
  total: number
  no_op: number
  created: number
  removed: number
  adopted: number
  blocked_conflict: number
  blocked_broken: number
  ownership_lost: number
  failed: number
}

export interface DistributionResult {
  target_id: number
  dry_run: boolean
  outcome: 'succeeded' | 'partial' | 'blocked' | 'failed'
  error?: string
  inspected_at?: string
  stale: boolean
  items: DistributionItemResult[]
  summary: DistributionSummary
}

export interface AdoptResult {
  target_id: number
  skill_id: number
  slug: string
  link_path: string
  raw_target: string
  adopted_at: string
  result: string
}

function postDistribution<T>(
  path: string,
  fallbackKey: DictionaryKey,
  body: object,
  signal?: AbortSignal,
): Promise<T> {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, fallbackKey, { status: res.status })
    return (await res.json()) as T
  })
}

export function inspectTarget(targetId: number | string, signal?: AbortSignal): Promise<DistributionStatus> {
  return postDistribution(`/api/v1/targets/${targetId}/inspect`, 'errInspectingTargetFailed', {}, signal)
}

export function distributeTarget(
  targetId: number | string,
  dryRun: boolean,
  signal?: AbortSignal,
): Promise<DistributionResult> {
  return postDistribution(
    `/api/v1/targets/${targetId}/distribute`,
    'errDistributingTargetFailed',
    { dry_run: dryRun },
    signal,
  )
}

export function adoptTargetLink(
  targetId: number | string,
  skillId: number,
  signal?: AbortSignal,
): Promise<AdoptResult> {
  return postDistribution(`/api/v1/targets/${targetId}/adopt`, 'errAdoptingLinkFailed', { skill_id: skillId }, signal)
}

export function linkTargetSkill(
  targetId: number | string,
  skillId: number,
  signal?: AbortSignal,
): Promise<DistributionStatus> {
  return postDistribution(`/api/v1/targets/${targetId}/link`, 'errLinkingSkillFailed', { skill_id: skillId }, signal)
}

export function unlinkTargetSkill(
  targetId: number | string,
  skillId: number,
  signal?: AbortSignal,
): Promise<DistributionStatus> {
  return postDistribution(`/api/v1/targets/${targetId}/unlink`, 'errUnlinkingSkillFailed', { skill_id: skillId }, signal)
}
