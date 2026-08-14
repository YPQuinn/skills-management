// The Source batch synchronization section: one button triggers
// POST /api/v1/sources/{id}/sync and the per-item outcomes plus the
// summary tally render below, linking each Skill to its Synchronization
// tab.
import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ArrowsExchange } from '@appica/icons-react'
import { syncSourceSkills } from './sync-api'
import type { SyncBatchResult } from './sync-api'
import { SyncResultBadge, SyncStatusBadge } from './sync-status'
import { useLocale } from './locale-context'

interface SourceSyncSectionProps {
  sourceId: number
  onSynced?: () => void
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SourceSyncSection({ sourceId, onSynced }: SourceSyncSectionProps) {
  const { t, getErrorMessage } = useLocale()
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [result, setResult] = useState<SyncBatchResult | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const run = async () => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setSyncing(true)
    setError(null)
    try {
      const data = await syncSourceSkills(sourceId, controller.signal)
      if (inflight.current === controller) setResult(data)
      onSynced?.()
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (inflight.current === controller) setError(err)
    } finally {
      if (inflight.current === controller) setSyncing(false)
    }
  }

  return (
    <section aria-labelledby="source-sync-heading" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="source-sync-heading" className="text-lg font-semibold">
            {t('sourceSyncHeading')}
          </h2>
          <p className="text-sm text-foreground-subtle">{t('sourceSyncDesc')}</p>
        </div>
        <Button onClick={() => void run()} disabled={syncing} focusableWhenDisabled>
          {syncing ? (
            <Spinner data-icon="start" currentColor className="text-[1.2em]" />
          ) : (
            <ArrowsExchange data-icon="start" />
          )}
          {syncing ? t('btnSyncingBoundSkills') : t('btnSyncBoundSkills')}
        </Button>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertSyncSourceFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errSyncingSourceFailed')}</AlertDescription>
        </Alert>
      )}

      {result !== null && (
        <div className="space-y-3">
          <Alert variant="success">
            <AlertTitle>{t('batchSyncCompleted')}</AlertTitle>
            <AlertDescription>{t('batchSyncSummary', { ...result.summary })}</AlertDescription>
          </Alert>
          {result.items.length === 0 ? (
            <p className="text-sm text-foreground-subtle">{t('batchNoBoundSkills')}</p>
          ) : (
            <div className="border border-border rounded-xl overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('colSyncItemSkill')}</TableHead>
                    <TableHead>{t('colSyncItemStatus')}</TableHead>
                    <TableHead>{t('colSyncItemResult')}</TableHead>
                    <TableHead>{t('colSyncItemDetail')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {result.items.map((item) => (
                    <TableRow key={item.skill_id}>
                      <TableCell className="font-medium">
                        <Link
                          to={`/skills/${encodeURIComponent(item.slug)}?tab=synchronization`}
                          className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                        >
                          {item.slug}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <SyncStatusBadge status={item.status} />
                      </TableCell>
                      <TableCell>
                        <SyncResultBadge result={item.result} />
                      </TableCell>
                      <TableCell className="text-foreground-subtle text-xs max-w-72 break-words">
                        {item.message || '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
