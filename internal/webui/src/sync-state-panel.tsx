// The Sync Status card, the Latest Sync Action card, and the staleness,
// action-error, and outcome alerts of the Skill Synchronization tab.
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { SyncStatusBadge, SyncResultBadge } from './sync-status'
import { syncActionKey } from './sync-status-policy'
import type { Skill } from './skill-api'
import type { SyncItemResult } from './sync-api'
import { useLocale } from './locale-context'

interface SyncStatePanelProps {
  skill: Skill
  outcome: SyncItemResult | null
  actionError: unknown | null
}

function outcomeVariant(result: string): 'success' | 'warning' | 'error' {
  switch (result) {
    case 'updated':
    case 'kept_store':
    case 'accepted_source':
    case 'rolled_back':
      return 'success'
    case 'no_op':
    case 'skipped':
    case 'blocked':
      return 'warning'
    default:
      return 'error'
  }
}

export function SyncStatePanel({ skill, outcome, actionError }: SyncStatePanelProps) {
  const { t, formatTime, getErrorMessage } = useLocale()
  const last = skill.last_sync
  const actionLabel = last && syncActionKey[last.action] ? t(syncActionKey[last.action]) : (last?.action ?? '')

  return (
    <div className="space-y-4">
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
          <div>
            <div className="text-foreground-muted">{t('labelStoreDigest')}</div>
            <div className="font-mono text-xs break-all mt-0.5">{skill.store_digest || '—'}</div>
          </div>
          <div>
            <div className="text-foreground-muted">{t('labelBaselineDigest')}</div>
            <div className="font-mono text-xs break-all mt-0.5">{skill.baseline_digest || '—'}</div>
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
                <div>
                  <div className="text-foreground-muted">{t('labelSyncBeforeDigest')}</div>
                  <div className="font-mono text-xs break-all mt-0.5">{last.before_digest || '—'}</div>
                </div>
                <div>
                  <div className="text-foreground-muted">{t('labelSyncAfterDigest')}</div>
                  <div className="font-mono text-xs break-all mt-0.5">{last.after_digest || '—'}</div>
                </div>
                <div>
                  <div className="text-foreground-muted">{t('labelSyncRevision')}</div>
                  <div className="font-mono text-xs break-all mt-0.5">{last.revision || '—'}</div>
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
