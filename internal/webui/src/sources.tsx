import { useCallback, useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchSources } from './source-api'
import type { SourceSummary } from './source-api'
import { SourceStatusBadge } from './source-status'
import { AddSourceForm } from './add-source-form'
import { SourceDetailPage } from './source-detail'
import { useLocale } from './locale-context'

export { SourceStatusBadge } from './source-status'
export type { SourceDetail, SourceEntry, SourceIssue, SourceSummary } from './source-api'

function truncate(value: string, max = 48): string {
  return value.length > max ? value.slice(0, max - 1) + '…' : value
}

export function SourcesIndex() {
  const { t, formatTime, getErrorMessage } = useLocale()
  const [sources, setSources] = useState<SourceSummary[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)

  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
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
  }, [])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t('sourcesTitle')}</h1>
          <p className="text-foreground-muted text-sm">{t('sourcesSubtitle')}</p>
        </div>
      </div>

      <AddSourceForm />

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadSources')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadSources')}</AlertDescription>
        </Alert>
      )}

      {loading && sources === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingSources')} />
        </div>
      ) : sources === null ? null : sources.length === 0 ? (
        <p className="text-foreground-muted">{t('emptySourcesIndex')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[800px]">
            <Table aria-label={t('captionSourcesIndex')}>
              <TableCaption className="sr-only">{t('captionSourcesIndex')}</TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead>{t('colKind')}</TableHead>
                  <TableHead>{t('colLocation')}</TableHead>
                  <TableHead>{t('colStatus')}</TableHead>
                  <TableHead className="text-end">{t('colSkills')}</TableHead>
                  <TableHead>{t('colLastChecked')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sources.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link to={`/sources/${encodeURIComponent(s.name)}`} className="underline decoration-border underline-offset-2 hover:decoration-foreground">
                        {s.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge variant="soft">{s.kind}</Badge>
                    </TableCell>
                    <TableCell className="text-foreground-muted" title={s.location}>{truncate(s.location)}</TableCell>
                    <TableCell>
                      <SourceStatusBadge available={s.available} stale={s.stale} />
                    </TableCell>
                    <TableCell className="text-end tabular-nums">{s.entry_count}</TableCell>
                    <TableCell className="text-foreground-muted">{formatTime(s.last_checked_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </ScrollArea>
      )}
    </div>
  )
}

export function SourceExplorer() {
  const { name } = useParams()
  return <SourceDetailPage key={name} />
}
