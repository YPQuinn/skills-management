// Source REST/JSON boundary: the resource shapes returned by /api/v1/sources
// and the request helpers every Source view shares. The REST adapter owns
// the same snake_case field names, so these types mirror the Go DTOs rather
// than inventing a camelCase view layer.
export interface SourceEntry {
  relative_dir: string
  name: string
  description: string
}

export interface SourceIssue {
  relative_dir: string
  reason: string
}

export interface SourceSummary {
  id: number
  name: string
  kind: 'local' | 'git'
  location: string
  ref?: string
  subpath?: string
  available: boolean
  stale: boolean
  last_error?: string
  last_checked_at?: string
  last_successful_check_at?: string
  last_commit?: string
  entry_count: number
}

export interface SourceDetail extends SourceSummary {
  created_at: string
  updated_at: string
  inventory: SourceEntry[]
  issues: SourceIssue[]
}

interface ErrorEnvelope {
  error?: { message?: string }
}

export function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

// responseError converts a failed response into the server's envelope
// message, or a stable fallback when the body is not an envelope.
async function responseError(res: Response, fallback: string): Promise<Error> {
  const data = (await res.json()) as ErrorEnvelope
  return new Error(data?.error?.message || fallback)
}

export function fetchSources(signal?: AbortSignal): Promise<SourceSummary[]> {
  return fetch('/api/v1/sources', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, `Server responded with ${res.status}`)
      return (await res.json()) as { items: SourceSummary[] }
    })
    .then((data) => data.items)
}

export function createSource(body: Record<string, string>, signal?: AbortSignal): Promise<SourceDetail> {
  return fetch('/api/v1/sources', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'Registering the Source failed')
    return (await res.json()) as SourceDetail
  })
}
