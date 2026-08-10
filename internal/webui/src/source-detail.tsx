import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { Spinner } from '@appica/ui-react/spinner'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { SourceStatusBadge } from './source-status'
import type { SourceDetail, SourceSummary } from './source-api'

interface ErrorEnvelope {
  error?: { message?: string }
}

function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

function formatTime(value?: string): string {
  if (!value) return 'never'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

function truncate(value: string, max = 80): string {
  return value.length > max ? value.slice(0, max - 1) + '…' : value
}

export function SourceDetailPage({ onCheckSuccess }: { onCheckSuccess?: () => void }) {
  const { name } = useParams()
  const [source, setSource] = useState<SourceDetail | null>(null)
  const [sourceId, setSourceId] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)
  // Any in-flight load or check is aborted before a new request starts and
  // on unmount, so a stale response can never overwrite the current route.
  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    fetch('/api/v1/sources', { signal: controller.signal })
      .then(async (res) => {
        if (!res.ok) {
          const data = (await res.json()) as ErrorEnvelope
          throw new Error(data?.error?.message || `Server responded with ${res.status}`)
        }
        return res.json()
      })
      .then((data: { items: SourceSummary[] }) => {
        const found = data.items.find(s => s.name === name)
        if (!found) throw new Error('Source not found')
        if (inflight.current === controller) setSourceId(found.id)
        return fetch(`/api/v1/sources/${found.id}`, { signal: controller.signal })
      })
      .then(async (res) => {
        if (!res.ok) {
          const data = (await res.json()) as ErrorEnvelope
          throw new Error(data?.error?.message || `Server responded with ${res.status}`)
        }
        return res.json()
      })
      .then((data: SourceDetail) => {
        if (inflight.current === controller) setSource(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(getErrorMessage(err))
      })
    return controller
  }, [name])

  useEffect(() => {
    // reset loading/error state for the current route; the previous Source
    // must never linger as the new selection while it reloads
    setSource(null)
    setError(null)
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  const check = async () => {
    if (sourceId === null) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setChecking(true)
    setError(null)
    try {
      const res = await fetch(`/api/v1/sources/${sourceId}/check`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
        signal: controller.signal,
      })
      if (!res.ok) {
        const data = (await res.json()) as ErrorEnvelope
        throw new Error(data?.error?.message || 'Checking the Source failed')
      }
      const data = (await res.json()) as SourceDetail
      if (inflight.current === controller) {
        setSource(data)
        onCheckSuccess?.()
      }
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (inflight.current === controller) setError(getErrorMessage(err))
    } finally {
      if (inflight.current === controller) setChecking(false)
    }
  }

  if (error && !source) {
    return (
      <Alert variant="error">
        <AlertTitle>Could not load Source</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }
  if (!source) {
    return <Spinner className="text-3xl text-foreground-subtle" aria-label="Loading source" />
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <Link to="/sources" className="text-sm text-foreground-subtle underline decoration-border underline-offset-2 hover:decoration-foreground">
            ← All Sources
          </Link>
          <h1 className="text-2xl font-bold mt-1">{source.name}</h1>
          <p className="text-foreground-subtle text-sm break-all">{source.location}</p>
        </div>
        <div className="flex flex-col items-end gap-2">
          <SourceStatusBadge available={source.available} stale={source.stale} />
          <Button onClick={check} disabled={checking} focusableWhenDisabled>
            {checking && <Spinner data-icon="start" currentColor />}
            {checking ? 'Checking…' : 'Check again'}
          </Button>
        </div>
      </div>

      {error && (
        <Alert variant="error">
          <AlertTitle>Check failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!source.available && source.last_error && (
        <Alert variant="error">
          <AlertTitle>Source unavailable</AlertTitle>
          <AlertDescription>
            {source.last_error}. The last successful check was {formatTime(source.last_successful_check_at)}; the
            Inventory below is stale.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 sm:grid-cols-3 text-sm">
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">Kind</div>
          <div className="font-medium mt-0.5">{source.kind}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">Ref</div>
          <div className="font-medium mt-0.5">{source.ref || 'default branch'}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">Subpath</div>
          <div className="font-medium mt-0.5">{source.subpath || 'Source root'}</div>
        </div>
        {source.kind === 'git' && (
          <div className="border border-border rounded-xl p-4 bg-background">
            <div className="text-foreground-subtle">Resolved commit</div>
            <div className="font-medium mt-0.5 font-mono break-all">{source.last_commit || '—'}</div>
          </div>
        )}
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">Last checked</div>
          <div className="font-medium mt-0.5">{formatTime(source.last_checked_at)}</div>
        </div>
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">Inventory ({source.inventory.length})</h2>
        {source.inventory.length === 0 ? (
          <p className="text-foreground-subtle">No valid Skills discovered.</p>
        ) : (
          <ScrollArea className="w-full" orientation="horizontal">
            <div className="min-w-[600px]">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Skill</TableHead>
                    <TableHead>Directory</TableHead>
                    <TableHead>Description</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {source.inventory.map((e) => (
                    <TableRow key={e.relative_dir}>
                      <TableCell className="font-medium text-foreground-strong">{e.name}</TableCell>
                      <TableCell className="font-mono text-foreground-subtle">{e.relative_dir}</TableCell>
                      <TableCell className="text-foreground-subtle">{truncate(e.description)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </ScrollArea>
        )}
      </div>

      {source.issues.length > 0 && (
        <div>
          <Alert variant="warning">
            <AlertTitle>{source.issues.length} invalid entr{source.issues.length === 1 ? 'y' : 'ies'} skipped</AlertTitle>
            <AlertDescription>
              These directories look like Skills but failed validation; they are reported and excluded from the
              Inventory.
            </AlertDescription>
          </Alert>
          <ul className="list-disc pl-4 mt-2 space-y-1">
            {source.issues.map((i) => (
              <li key={i.relative_dir}>
                <span className="font-mono">{i.relative_dir}</span>: {i.reason}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
