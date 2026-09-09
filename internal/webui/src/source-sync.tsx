// The Source's batch synchronization. The action sits in the Inventory
// toolbar beside Rescan — both are things you do to this Source — while
// everything the run has to say renders as one block below that toolbar:
// the bound Skills' Sync Status, a Conflict that blocks it, and the
// outcomes of the last run. Splitting the state into a hook is what lets
// one POST drive a button and a status block in two different rows.
import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertIcon, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'
import { ArrowsExchange, AlertTriangleFilled } from '@appica/icons-react'
import { syncSourceSkills } from './sync-api'
import type { SyncActionResult, SyncBatchResult } from './sync-api'
import { SyncResultBadge, SyncStatusBadge } from './sync-status'
import { syncResultNeedsAttention } from './sync-status-policy'
import { summarizeBoundSync } from './source-sync-summary'
import { RelativeTime } from './relative-time'
import { useNotifySuccess } from './notify-success'
import type { Skill } from './skill-api'
import { useLocale } from './locale-context'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export interface SourceSyncState {
  syncing: boolean
  error: unknown | null
  run: () => void
}

// A run's outcomes are news, not state: they describe a moment that has
// already passed. The toast carries them, and what the run left behind is
// read back off the refreshed Skills instead.
export function useSourceSync(sourceId: number | null, onSynced?: () => void): SourceSyncState {
  const { t } = useLocale()
  const notifySuccess = useNotifySuccess()
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const run = () => {
    if (sourceId === null) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setSyncing(true)
    setError(null)
    void (async () => {
      try {
        const data = await syncSourceSkills(sourceId, controller.signal)
        if (inflight.current === controller) reportRun(data, t, notifySuccess)
        onSynced?.()
      } catch (err: unknown) {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(err)
      } finally {
        if (inflight.current === controller) setSyncing(false)
      }
    })()
  }

  return { syncing, error, run }
}

// A toast this wide holds a few rows before it becomes a wall; the Skills
// index is the place to read a long list.
const MAX_TOAST_ITEMS = 5

// Best outcome first, so a run's headline is what moved rather than what
// did not. Empty buckets are dropped — in a 360px toast a run of zeros
// costs three lines and says nothing.
const RESULT_TALLY_ORDER: SyncActionResult[] = [
  'updated',
  'accepted_source',
  'kept_store',
  'rolled_back',
  'no_op',
  'skipped',
  'blocked',
  'failed',
]

function reportRun(
  data: SyncBatchResult,
  t: ReturnType<typeof useLocale>['t'],
  notifySuccess: ReturnType<typeof useNotifySuccess>,
) {
  const attention = data.items.filter((item) => syncResultNeedsAttention(item.result))
  const shown = attention.slice(0, MAX_TOAST_ITEMS)
  const buckets = RESULT_TALLY_ORDER.filter((result) => data.summary[result] > 0)

  notifySuccess(t('toastSynced'), {
    // Anything asking the reader to act has to outlast a five-second glance.
    ...(attention.length > 0 ? { timeout: 0 } : {}),
    description: (
      <div className="space-y-2">
        {data.items.length === 0 ? (
          <p>{t('batchNoBoundSkills')}</p>
        ) : (
          <div className="flex flex-wrap items-center gap-1.5">
            {buckets.map((result) => (
              <SyncResultBadge
                key={result}
                result={result}
                count={data.summary[result]}
                size="sm"
              />
            ))}
          </div>
        )}
        {shown.length > 0 && (
          <ul className="space-y-1.5">
            {shown.map((item) => (
              <li key={item.skill_id} className="flex flex-wrap items-center gap-2">
                <Link
                  to={`/skills/${encodeURIComponent(item.slug)}?tab=synchronization`}
                  className="font-medium text-foreground underline underline-offset-2"
                >
                  {item.slug}
                </Link>
                <SyncStatusBadge status={item.status} size="sm" />
                {item.message && <span className="text-xs">{item.message}</span>}
              </li>
            ))}
          </ul>
        )}
        {attention.length > shown.length && (
          <p>{t('batchSyncMoreItems', { count: attention.length - shown.length })}</p>
        )}
      </div>
    ),
  })
}

export function SourceSyncButton({
  syncing,
  onRun,
  lastEvaluatedAt,
}: {
  syncing: boolean
  onRun: () => void
  lastEvaluatedAt?: string
}) {
  const { t } = useLocale()
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button size="sm" onClick={onRun} disabled={syncing} focusableWhenDisabled>
            {syncing ? (
              <Spinner data-icon="start" currentColor className="text-[1.2em]" />
            ) : (
              <ArrowsExchange data-icon="start" />
            )}
            {syncing ? t('btnSyncingBoundSkills') : t('btnSyncBoundSkills')}
          </Button>
        }
      />
      <TooltipContent className="max-w-xs">
        {t('sourceSyncDesc')}
        <div className="mt-1 opacity-80">
          {t('sourceSyncLastEvaluated')}: <RelativeTime value={lastEvaluatedAt} />
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

export function SourceSyncStatus({
  boundSkills,
  error,
}: {
  boundSkills: Skill[]
  error: unknown | null
}) {
  const { t, getErrorMessage } = useLocale()
  const summary = summarizeBoundSync(boundSkills)
  const allInSync =
    summary.total > 0 && summary.byStatus.length === 1 && summary.byStatus[0].status === 'in_sync'
  // Conflict is left to its Alert, which also says how to resolve it, and
  // the settled states carry no news worth a badge.
  const needsAttention = summary.byStatus.filter(
    ({ status }) => status !== 'conflict' && status !== 'in_sync' && status !== 'unbound',
  )

  return (
    <div className="space-y-3">
      {summary.total === 0 ? (
        <p className="text-sm text-foreground-muted">{t('sourceSyncNoneBound')}</p>
      ) : allInSync ? (
        <p className="text-sm text-foreground-muted">
          {t('sourceSyncAllInSync', { count: summary.total })}
        </p>
      ) : (
        needsAttention.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            {needsAttention.map(({ status, count }) => (
              <SyncStatusBadge key={status} status={status} count={count} size="sm" />
            ))}
          </div>
        )
      )}

      {summary.conflicts > 0 && (
        <Alert layout="inline" variant="error">
          <AlertIcon>
            <AlertTriangleFilled />
          </AlertIcon>
          <AlertTitle>{t('alertSyncConflict')}</AlertTitle>
          <AlertDescription>
            {t('sourceSyncConflictDesc', { count: summary.conflicts })}
          </AlertDescription>
        </Alert>
      )}

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertSyncSourceFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errSyncingSourceFailed')}</AlertDescription>
        </Alert>
      )}

    </div>
  )
}
