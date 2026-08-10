import { useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchSources, getErrorMessage } from './source-api'
import type { SourceSummary } from './source-api'
import { SourceStatusBadge } from './source-status'

// SourceListPane is the compact master–detail companion to SourceDetailPage:
// concise name, status, entry count, and location context with links to the
// stable /sources/:name route. Registration and the full table live on the
// /sources index, never in this pane.
export function SourceListPane({ refreshCounter = 0 }: { refreshCounter?: number }) {
  const { name } = useParams()
  const [sources, setSources] = useState<SourceSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const inflight = useRef<AbortController | null>(null)

  useEffect(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)
    fetchSources(controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSources(data)
      })
      .catch((err: unknown) => {
        if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
        if (inflight.current === controller) setError(getErrorMessage(err))
      })
      .finally(() => {
        if (inflight.current === controller) setLoading(false)
      })
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [refreshCounter])

  return (
    <nav aria-label="Source list" className="rounded-xl border border-border bg-background p-4">
      <h2 className="mb-3 text-sm font-semibold text-foreground-strong">Sources</h2>

      {error && (
        <Alert variant="error">
          <AlertTitle>Could not load Sources</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {loading && sources === null ? (
        <div className="flex justify-center py-8">
          <Spinner className="text-2xl text-foreground-subtle" aria-label="Loading sources" />
        </div>
      ) : sources === null ? null : sources.length === 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-foreground-subtle">No Sources registered yet.</p>
          <Link
            to="/sources"
            className="text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            Register a Source
          </Link>
        </div>
      ) : (
        <ul className="space-y-1">
          {sources.map((s) => {
            const selected = s.name === name
            return (
              <li key={s.id} className="min-w-0">
                <Link
                  to={`/sources/${encodeURIComponent(s.name)}`}
                  aria-current={selected ? 'page' : undefined}
                  className={`block min-w-0 rounded-lg px-3 py-2 hover:bg-foreground/5 ${
                    selected ? 'bg-foreground/10' : ''
                  }`}
                >
                  <span className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm font-medium text-foreground-strong">{s.name}</span>
                    <SourceStatusBadge available={s.available} stale={s.stale} />
                  </span>
                  <span className="mt-0.5 flex items-center justify-between gap-2">
                    <span className="truncate text-xs text-foreground-subtle" title={s.location}>{s.location}</span>
                    <span className="shrink-0 text-xs text-foreground-subtle tabular-nums">{s.entry_count} skills</span>
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </nav>
  )
}
