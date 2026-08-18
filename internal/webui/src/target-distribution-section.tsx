import { useCallback, useState } from 'react'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Link, Checklist, Refresh } from '@appica/icons-react'
import {
  inspectTarget,
  distributeTarget,
  adoptTargetLink,
  type DistributionStatus,
  type DistributionItemView,
  type DistributionResult,
} from './distribution-api'
import { useLocale } from './locale-context'
import { TargetDistributionDialogs } from './target-distribution-dialogs'
import { stateKey } from './target-distribution-labels'
import { TargetDistributionTable, type DistributionBusy } from './target-distribution-table'

interface TargetDistributionSectionProps {
  targetId: number
  initial: DistributionStatus | null
  onChanged: (status: DistributionStatus) => void
}

export function TargetDistributionSection({ targetId, initial, onChanged }: TargetDistributionSectionProps) {
  const { t, formatTime, getErrorMessage } = useLocale()
  const [status, setStatus] = useState<DistributionStatus | null>(initial)
  const [busy, setBusy] = useState<DistributionBusy>(null)
  const [actionError, setActionError] = useState<unknown | null>(null)
  const [preview, setPreview] = useState<DistributionResult | null>(null)
  const [result, setResult] = useState<DistributionResult | null>(null)
  const [confirmDistribute, setConfirmDistribute] = useState(false)
  const [adoptItem, setAdoptItem] = useState<DistributionItemView | null>(null)
  const [boundId, setBoundId] = useState(targetId)
  if (targetId !== boundId) {
    setBoundId(targetId)
    setStatus(initial)
    setBusy(null)
    setActionError(null)
    setPreview(null)
    setResult(null)
    setConfirmDistribute(false)
    setAdoptItem(null)
  }

  const run = useCallback(async (kind: Exclude<DistributionBusy, null>, fn: () => Promise<void>) => {
    setBusy(kind)
    setActionError(null)
    try {
      await fn()
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setBusy(null)
    }
  }, [])

  const handleInspect = () =>
    run('inspect', async () => {
      const st = await inspectTarget(targetId)
      setStatus(st)
      onChanged(st)
    })

  const handlePreview = () =>
    run('preview', async () => {
      const res = await distributeTarget(targetId, true)
      setStatus((prev) => (prev ? { ...prev, stale: res.stale } : prev))
      setPreview(res)
    })

  const handleDistribute = () =>
    run('distribute', async () => {
      const res = await distributeTarget(targetId, false)
      const st = await inspectTarget(targetId)
      setStatus(st)
      onChanged(st)
      setResult(res)
    })

  const handleAdopt = (item: DistributionItemView) =>
    run(`adopt-${item.skill_id}`, async () => {
      await adoptTargetLink(targetId, item.skill_id)
      const st = await inspectTarget(targetId)
      setStatus(st)
      onChanged(st)
      setAdoptItem(null)
    })

  const disabled = busy !== null
  const items = status?.items ?? []
  const state = status?.state ?? ''

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">{t('distributionHeading')}</h2>
          <p className="text-sm text-foreground-subtle">{t('distributionSubtitle')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {state !== '' && (
            <Badge variant={state === 'ok' ? 'soft' : state === 'missing' ? 'outline' : 'error'}>
              {t(stateKey(state))}
            </Badge>
          )}
          <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={handleInspect}>
            {busy === 'inspect' ? (
              <Spinner data-icon="start" currentColor className="text-[1.2em]" />
            ) : (
              <Refresh data-icon="start" />
            )}
            {busy === 'inspect' ? t('btnInspecting') : t('btnInspect')}
          </Button>
          <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={handlePreview}>
            {busy === 'preview' ? (
              <Spinner data-icon="start" currentColor className="text-[1.2em]" />
            ) : (
              <Checklist data-icon="start" />
            )}
            {busy === 'preview' ? t('btnPreviewing') : t('btnPreview')}
          </Button>
          <Button disabled={disabled} focusableWhenDisabled onClick={() => setConfirmDistribute(true)}>
            {busy === 'distribute' ? (
              <Spinner data-icon="start" currentColor className="text-[1.2em]" />
            ) : (
              <Link data-icon="start" />
            )}
            {busy === 'distribute' ? t('btnDistributing') : t('btnDistribute')}
          </Button>
        </div>
      </div>

      {status?.inspected_at && (
        <p className="text-xs text-foreground-subtle">
          {t('inspectedAtLabel', { time: formatTime(status.inspected_at) })}
          {status.last_completed_at && ` · ${t('distributionLastOutcome', { outcome: status.last_result })}`}
        </p>
      )}

      {actionError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertDistributionActionFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(actionError, 'errDistributingTargetFailed')}</AlertDescription>
        </Alert>
      )}

      {status?.stale && (
        <Alert variant="warning">
          <AlertTitle>{t('observedBrokenLink')}</AlertTitle>
          <AlertDescription>
            {t('staleObservation', { error: status.inspection_error || status.state_error || '' })}
          </AlertDescription>
        </Alert>
      )}

      <TargetDistributionTable items={items} busy={busy} disabled={disabled} onAdopt={setAdoptItem} />

      <TargetDistributionDialogs
        preview={preview}
        result={result}
        confirmDistribute={confirmDistribute}
        adoptItem={adoptItem}
        disabled={disabled}
        onClosePreview={() => setPreview(null)}
        onCloseResult={() => setResult(null)}
        onConfirmOpenChange={setConfirmDistribute}
        onCloseAdopt={() => setAdoptItem(null)}
        onConfirmDistribute={() => {
          setConfirmDistribute(false)
          handleDistribute()
        }}
        onConfirmAdopt={() => {
          const item = adoptItem
          setAdoptItem(null)
          if (item) handleAdopt(item)
        }}
      />
    </div>
  )
}
