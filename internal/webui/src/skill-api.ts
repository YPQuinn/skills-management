// Skill REST/JSON boundary: resource shapes returned by /api/v1/skills
// and /api/v1/skills/import. The REST adapter owns snake_case field names.
import { ApiError, type DictionaryKey } from './locale-dictionary'

export interface SkillBinding {
  source_id: number
  source_name: string
  relative_dir: string
  digest: string
  source_commit?: string
  imported_at: string
}

export interface Skill {
  id: number
  slug: string
  name: string
  description: string
  store_digest: string
  baseline_digest: string
  created_at: string
  updated_at: string
  binding?: SkillBinding
}

export interface SkillListResponse {
  items: Skill[]
  total: number
}

export interface ImportSelector {
  relative_dir?: string
  name?: string
  slug?: string
  replace?: boolean
}

export interface ImportRequest {
  source_id: number
  selectors?: ImportSelector[]
  all?: boolean
  allow_large?: boolean
}

export interface ImportReplacesInfo {
  skill_id: number
  slug: string
  name: string
}

export interface ImportImpactGroup {
  id: number
  name: string
}

export interface ImportImpactTarget {
  id: number
  name: string
  direct: boolean
  groups?: ImportImpactGroup[]
}

export interface ImportImpact {
  groups?: ImportImpactGroup[]
  targets?: ImportImpactTarget[]
}

export type ImportStatus = 'imported' | 'already_imported' | 'skipped_conflict' | 'replaced' | 'failed'

export interface ImportItemResult {
  status: ImportStatus
  relative_dir: string
  requested_slug?: string
  slug?: string
  skill_id?: number
  replaces?: ImportReplacesInfo
  impact?: ImportImpact
  code?: string
  message?: string
}

export interface ImportSummary {
  total: number
  imported: number
  already_imported: number
  skipped_conflict: number
  replaced: number
  failed: number
}

export interface ImportResponse {
  items: ImportItemResult[]
  summary: ImportSummary
}

interface ErrorEnvelope {
  error?: { message?: string; code?: string }
}

async function responseError(
  res: Response,
  fallbackKey: DictionaryKey,
  params?: Record<string, string | number>,
): Promise<ApiError> {
  let serverMessage: string | undefined
  try {
    const data = (await res.json()) as ErrorEnvelope
    if (data?.error?.message) {
      serverMessage = data.error.message
    }
  } catch {}
  return new ApiError(serverMessage, fallbackKey, params || { status: res.status })
}

export function fetchSkills(signal?: AbortSignal): Promise<Skill[]> {
  return fetch('/api/v1/skills', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, 'errServerResponded', { status: res.status })
      return (await res.json()) as SkillListResponse
    })
    .then((data) => data.items || [])
}

export function fetchSkill(idOrSlug: string | number, signal?: AbortSignal): Promise<Skill> {
  return fetchSkills(signal).then(async (skills) => {
    const found = skills.find((s) => s.slug === String(idOrSlug) || s.id === Number(idOrSlug))
    if (!found) throw new ApiError(undefined, 'errSkillNotFound')

    const res = await fetch(`/api/v1/skills/${found.id}`, { signal })
    if (!res.ok) throw await responseError(res, 'errServerResponded', { status: res.status })
    return (await res.json()) as Skill
  })
}

export function importSkills(body: ImportRequest, signal?: AbortSignal): Promise<ImportResponse> {
  return fetch('/api/v1/skills/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errImportingSkillsFailed')
    return (await res.json()) as ImportResponse
  })
}
