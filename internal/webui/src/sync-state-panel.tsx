// The Sync Status card, the Latest Sync Action card, and the staleness,
// action-error, and outcome alerts of the Skill Synchronization tab.
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@appica/ui-react/collapsible'
import { ChevronRight } from '@appica/icons-react'
import { CopyableField } from './copyable-path'
import { SyncStatusBadge, SyncResultBadge } from './sync-status'
import { isSuccessfulSyncResult, isSyncStatus, syncActionKey, syncConclusionKey } from './sync-status-policy'
import type { Skill } from './skill-api'
import type { SyncItemResult } from './sync-api'
import { useLocale } from './locale-context'
import { TermTooltip } from './term-tooltip'

interface SyncStatePanelProps {
  skill: Skill
  outcome: SyncItemResult | null
  actionError: unknown | null
}

function outcomeVariant(result: string): 'success' | 'warning' | 'error' {
  if (isSuccessfulSyncResult(result)) return 'success'
  if (result === 'failed') return 'error'
  return 'warning'
}

export function SyncStatePanel({ skill, outcome, actionError }: SyncStatePanelProps) {
  const { t, formatTime, getErrorMessage } = useLocale()
  const last = skill.last_sync
  const actionLabel = last && syncActionKey[last.action] ? t(syncActionKey[last.action]) : (last?.action ?? '')
  const status = skill.sync_status
  const conclusionKey = isSyncStatus(status) ? syncConclusionKey[status] : undefined
  const isConflict = status === 'conflict'

  return (
    <div className="space-y-4">
      {isConflict ? (
        <Alert variant="error">
          <AlertTitle>{t('alertSyncConflict')}</AlertTitle>
          <AlertDescription>{t('syncConclusionConflict')}</AlertDescription>
        </Alert>
      ) : (
        conclusionKey && <p className="text-sm text-foreground">{t(conclusionKey)}</p>
      )}

      <div className="grid gap-4 sm:grid-cols-2 text-sm">
        <div className="border border-border rounded-xl p-4 bg-background space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-foreground-muted">{t('labelSyncStatus')}</span>
            <SyncStatusBadge status={skill.sync_status} />
            {skill.sync_stale && <Badge variant="warning">{t('statusSyncStale')}</Badge>}
          </div>
          <div>
            <div className="text-foreground-muted">{t('labelSyncCheckedAt')}</div>
            <div className="font-medium mt-0.5">{formatTime(skill.sync_checked_at)}</div>
          </div>
        </div>

        <div className="border border-border rounded-xl p-4 bg-background space-y-3">
          <h2 className="text-lg font-semibold">{t('latestSyncTitle')}</h2>
          {last ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{actionLabel}</Badge>
                <SyncResultBadge result={last.result} />
              </div>
              <div className="grid gap-2">
                <div>
                  <div className="text-foreground-muted">{t('labelSyncStarted')}</div>
                  <div className="font-medium mt-0.5">{formatTime(last.started_at)}</div>
                </div>
                <div>
                  <div className="text-foreground-muted">{t('labelSyncCompleted')}</div>
                  <div className="font-medium mt-0.5">{formatTime(last.completed_at)}</div>
                </div>
              </div>
              {last.error && (
                <Alert variant="error">
                  <AlertTitle>{t('labelSyncError')}</AlertTitle>
                  <AlertDescription className="break-all">{last.error}</AlertDescription>
                </Alert>
              )}
            </>
          ) : (
            <p className="text-sm text-foreground-muted">{t('noSyncYet')}</p>
          )}
        </div>
      </div>

      <Collapsible>
        <CollapsibleTrigger
          type="button"
          className="group text-foreground-muted inline-flex items-center gap-1.5 text-sm font-medium hover:text-foreground"
        >
          <ChevronRight className="size-4 shrink-0 stroke-2 transition-transform duration-200 group-data-panel-open:rotate-90 motion-reduce:transition-none" />
          {t('digestDetails')}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="mt-3 grid gap-3 sm:grid-cols-2 text-sm">
            <CopyableField label={t('labelStoreDigest')} value={skill.store_digest} />
            <CopyableField
              label={<TermTooltip term={t('labelBaselineDigest')} tip={t('tipBaseline')} />}
              value={skill.baseline_digest}
            />
            {last && (
              <>
                <CopyableField label={t('labelSyncBeforeDigest')} value={last.before_digest} />
                <CopyableField label={t('labelSyncAfterDigest')} value={last.after_digest} />
                <CopyableField label={t('labelSyncRevision')} value={last.revision} />
              </>
            )}
          </div>
        </CollapsibleContent>
      </Collapsible>

      {skill.sync_stale && (
        <Alert variant="warning">
          <AlertTitle>{t('statusSyncStale')}</AlertTitle>
          <AlertDescription>{t('staleNotice')}</AlertDescription>
        </Alert>
      )}

      {actionError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertSyncActionFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(actionError, 'errSyncingSkillFailed')}</AlertDescription>
        </Alert>
      )}

      {outcome !== null && (
        <Alert variant={outcomeVariant(outcome.result)}>
          <AlertTitle>{t('alertSyncOutcome')}</AlertTitle>
          <AlertDescription className="space-y-1">
            <span className="inline-flex items-center gap-2">
              <SyncResultBadge result={outcome.result} />
              {outcome.status && <SyncStatusBadge status={outcome.status} />}
            </span>
            {outcome.message && <span className="block break-all">{outcome.message}</span>}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
