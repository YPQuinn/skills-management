import { useCallback, useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchSources, getErrorMessage } from './source-api'
import type { SourceSummary } from './source-api'
import { SourceStatusBadge } from './source-status'
import { AddSourceForm } from './add-source-form'
import { SourceListPane } from './source-list-pane'
import { SourceDetailPage } from './source-detail'

// Re-exports keep the public surface used by App.tsx and source-detail.tsx
// stable after the split into feature modules.
export { SourceStatusBadge } from './source-status'
export { SourceListPane } from './source-list-pane'
export type { SourceDetail, SourceEntry, SourceIssue, SourceSummary } from './source-api'

function formatTime(value?: string): string {
  if (!value) return 'never'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

function truncate(value: string, max = 48): string {
  return value.length > max ? value.slice(0, max - 1) + '…' : value
}

export function SourcesIndex() {
  const [sources, setSources] = useState<SourceSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
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
        if (inflight.current === controller) setError(getErrorMessage(err))
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
          <h1 className="text-2xl font-bold">Sources</h1>
          <p className="text-foreground-subtle text-sm">Upstream locations Skills are imported and checked from.</p>
        </div>
      </div>

      <AddSourceForm />

      {error && (
        <Alert variant="error">
          <AlertTitle>Could not load Sources</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {loading && sources === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-subtle" aria-label="Loading sources" />
        </div>
      ) : sources === null ? null : sources.length === 0 ? (
        <p className="text-foreground-subtle">No Sources registered yet.</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[800px]">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Kind</TableHead>
                  <TableHead>Location</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-end">Skills</TableHead>
                  <TableHead>Last checked</TableHead>
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
                    <TableCell className="text-foreground-subtle" title={s.location}>{truncate(s.location)}</TableCell>
                    <TableCell>
                      <SourceStatusBadge available={s.available} stale={s.stale} />
                    </TableCell>
                    <TableCell className="text-end tabular-nums">{s.entry_count}</TableCell>
                    <TableCell className="text-foreground-subtle">{formatTime(s.last_checked_at)}</TableCell>
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

// SourceExplorer is the desktop resource-explorer master–detail view: a
// compact Source list stays visible beside the selected Source. On narrow
// screens the list pane is hidden and the detail is its own route view.
export function SourceExplorer() {
  const { name } = useParams()
  const [refreshCounter, setRefreshCounter] = useState(0)
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)] lg:items-start">
      <div className="hidden lg:block">
        <SourceListPane refreshCounter={refreshCounter} />
      </div>
      {/* keying by name remounts the detail on route change so the previous
          Source never renders as the new selection and an older response
          can never overwrite the current route */}
      <SourceDetailPage key={name} onCheckSuccess={() => setRefreshCounter(c => c + 1)} />
    </div>
  )
}
