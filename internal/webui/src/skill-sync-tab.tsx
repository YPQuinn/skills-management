// The Skill Synchronization tab: the state panel, the legal actions, and
// the three-way path diff. Every content-changing action refreshes the
// Skill and the diff afterwards; dialog actions report errors inside
// their Alert Dialog while check/sync errors surface inline.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { fetchSkill } from './skill-api'
import type { Skill } from './skill-api'
import {
  acceptSource,
  checkSkillSync,
  fetchSkillDiff,
  keepStore,
  rollbackSkill,
  syncSkill,
} from './sync-api'
import type { DiffResult, SyncItemResult } from './sync-api'
import { SyncStatePanel } from './sync-state-panel'
import { SyncActions } from './sync-actions'
import type { SyncActionName, SyncDialogName } from './sync-action-policy'
import { SyncDiff } from './sync-diff'
import { isSuccessfulSyncResult } from './sync-status-policy'
import { useLocale } from './locale-context'
import { useNotifySuccess } from './notify-success'

interface SkillSyncTabProps {
  skill: Skill
  onSkillUpdated: (skill: Skill) => void
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillSyncTab({ skill, onSkillUpdated }: SkillSyncTabProps) {
  const { t, getErrorMessage } = useLocale()
  const notifySuccess = useNotifySuccess()
  const [dialog, setDialog] = useState<SyncDialogName | null>(null)
  const [busy, setBusy] = useState<SyncActionName | null>(null)
  const [dialogError, setDialogError] = useState<unknown | null>(null)
  const [actionError, setActionError] = useState<unknown | null>(null)
  const [outcome, setOutcome] = useState<SyncItemResult | null>(null)
  const [diff, setDiff] = useState<DiffResult | null>(null)
  const [diffLoading, setDiffLoading] = useState(false)
  const [diffError, setDiffError] = useState<unknown | null>(null)
  const [pathFilter, setPathFilter] = useState('')
  const diffInflight = useRef<AbortController | null>(null)

  const loadDiff = useCallback(
    (path: string) => {
      diffInflight.current?.abort()
      const controller = new AbortController()
      diffInflight.current = controller
      setDiffLoading(true)
      setDiffError(null)
      fetchSkillDiff(skill.id, path, controller.signal)
        .then((data) => {
          if (diffInflight.current === controller) setDiff(data)
        })
        .catch((err: unknown) => {
          if (isAbortError(err)) return
          if (diffInflight.current === controller) setDiffError(err)
        })
        .finally(() => {
          if (diffInflight.current === controller) setDiffLoading(false)
        })
    },
    [skill.id],
  )

  useEffect(() => {
    if (skill.binding) loadDiff('')
    return () => {
      diffInflight.current?.abort()
      diffInflight.current = null
    }
  }, [loadDiff, skill.binding])

  const runAction = async (action: SyncActionName) => {
    setBusy(action)
    setActionError(null)
    setDialogError(null)
    try {
      if (action === 'check') {
        onSkillUpdated(await checkSkillSync(skill.id))
      } else {
        const item =
          action === 'sync'
            ? await syncSkill(skill.id)
            : action === 'keep_store'
              ? await keepStore(skill.id)
              : action === 'accept_source'
                ? await acceptSource(skill.id)
                : await rollbackSkill(skill.id)
        setOutcome(item)
        if (isSuccessfulSyncResult(item.result)) notifySuccess(t('toastSynced'))
        fetchSkill(skill.id)
          .then(onSkillUpdated)
          .catch(() => {})
        if (dialog === action) setDialog(null)
      }
      loadDiff(pathFilter)
    } catch (err: unknown) {
      if (action === 'keep_store' || action === 'accept_source' || action === 'rollback') {
        setDialogError(err)
      } else {
        setActionError(err)
      }
    } finally {
      setBusy(null)
    }
  }

  const handleCloseDialog = () => {
    if (busy !== null) return
    setDialog(null)
    setDialogError(null)
  }

  const handlePathFilterChange = (value: string) => {
    setPathFilter(value)
    loadDiff(value)
  }

  if (!skill.binding) {
    return (
      <div className="space-y-4">
        <Alert variant="info">
          <AlertTitle>{t('alertSyncUnbound')}</AlertTitle>
          <AlertDescription>{t('syncUnboundText')}</AlertDescription>
        </Alert>
        {actionError !== null && (
          <Alert variant="error">
            <AlertTitle>{t('alertSyncActionFailed')}</AlertTitle>
            <AlertDescription>{getErrorMessage(actionError, 'errCheckingSkillSyncFailed')}</AlertDescription>
          </Alert>
        )}
        <SyncActions
          skill={skill}
          dialog={dialog}
          busy={busy}
          dialogError={dialogError}
          onOpenDialog={setDialog}
          onCloseDialog={handleCloseDialog}
          onConfirm={(action) => {
            void runAction(action)
          }}
        />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <SyncStatePanel skill={skill} outcome={outcome} actionError={actionError} />

      <SyncActions
        skill={skill}
        dialog={dialog}
        busy={busy}
        dialogError={dialogError}
        onOpenDialog={setDialog}
        onCloseDialog={handleCloseDialog}
        onConfirm={(action) => {
          void runAction(action)
        }}
      />

      <SyncDiff
        diff={diff}
        loading={diffLoading}
        error={diffError}
        pathFilter={pathFilter}
        onPathFilterChange={handlePathFilterChange}
      />
    </div>
  )
}
