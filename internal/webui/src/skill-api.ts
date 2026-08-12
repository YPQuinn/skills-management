// Skill REST/JSON boundary: resource shapes returned by /api/v1/skills
// and /api/v1/skills/import. The REST adapter owns snake_case field names.

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

export interface ImportItemResult {
  status: 'imported' | 'skipped_imported' | 'skipped_conflict' | 'error' | string
  relative_dir: string
  requested_slug?: string
  slug?: string
  skill_id?: number
  replaces?: ImportReplacesInfo
  code?: string
  message?: string
}

export interface ImportSummary {
  imported?: number
  skipped?: number
  failed?: number
  [key: string]: unknown
}

export interface ImportResponse {
  items: ImportItemResult[]
  summary: ImportSummary
}

interface ErrorEnvelope {
  error?: { message?: string; code?: string }
}

export function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

async function responseError(res: Response, fallback: string): Promise<Error> {
  let message = fallback
  try {
    const data = (await res.json()) as ErrorEnvelope
    if (data?.error?.message) {
      message = data.error.message
    }
  } catch {
    // Keep fallback HTTP status message
  }
  return new Error(message)
}

export function fetchSkills(signal?: AbortSignal): Promise<Skill[]> {
  return fetch('/api/v1/skills', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, `Server responded with ${res.status}`)
      return (await res.json()) as SkillListResponse
    })
    .then((data) => data.items || [])
}

export function fetchSkill(idOrSlug: string | number, signal?: AbortSignal): Promise<Skill> {
  // If idOrSlug is string slug, first fetch skills list to find matching skill ID or slug
  return fetchSkills(signal).then(async (skills) => {
    const found = skills.find((s) => s.slug === String(idOrSlug) || s.id === Number(idOrSlug))
    if (!found) throw new Error('Skill not found')
    
    // Fetch detail endpoint by ID
    const res = await fetch(`/api/v1/skills/${found.id}`, { signal })
    if (!res.ok) throw await responseError(res, `Server responded with ${res.status}`)
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
    if (!res.ok) throw await responseError(res, 'Importing skills failed')
    return (await res.json()) as ImportResponse
  })
}
