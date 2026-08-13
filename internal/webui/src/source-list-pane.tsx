import { useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchSources } from './source-api'
import type { SourceSummary } from './source-api'
import { SourceStatusBadge } from './source-status'
import { useLocale } from './locale-context'

export function SourceListPane({ refreshCounter = 0 }: { refreshCounter?: number }) {
  const { name } = useParams()
  const { t, getErrorMessage } = useLocale()
  const [sources, setSources] = useState<SourceSummary[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
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
        if (inflight.current === controller) setError(err)
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
    <nav aria-label={t('ariaSourceList')} className="rounded-xl border border-border bg-background p-4">
      <h2 className="mb-3 text-sm font-semibold text-foreground-strong">{t('sourcesTitle')}</h2>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadSources')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {loading && sources === null ? (
        <div className="flex justify-center py-8">
          <Spinner className="text-2xl text-foreground-subtle" aria-label={t('ariaLoadingSources')} />
        </div>
      ) : sources === null ? null : sources.length === 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-foreground-subtle">{t('emptySourcesListPane')}</p>
          <Link
            to="/sources"
            className="text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {t('linkRegisterSource')}
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
                    <span className="shrink-0 text-xs text-foreground-subtle tabular-nums">
                      {t('countSkills', { count: s.entry_count })}
                    </span>
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
