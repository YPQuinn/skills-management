// The synchronization action bar plus one dedicated Appica Alert Dialog
// per consequential action. Each dialog names its own consequence, so no
// generic confirmation can ever stand in for Accept Source or Rollback.
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import { ArrowBackUp, ArrowDown, ArrowsExchange, Refresh, Shield } from '@appica/icons-react'
import type { Skill } from './skill-api'
import { syncActionsFor } from './sync-action-policy'
import type { SyncActionName, SyncDialogName } from './sync-action-policy'
import { useLocale } from './locale-context'
import type { DictionaryKey } from './locale-dictionary'

interface DialogDef {
  action: SyncDialogName
  titleKey: DictionaryKey
  descKey: DictionaryKey
  confirmKey: DictionaryKey
  pendingKey: DictionaryKey
  errorKey: DictionaryKey
  icon: typeof ArrowDown
  confirmVariant: 'primary' | 'destructive'
}

const DIALOG_DEFS: DialogDef[] = [
  {
    action: 'keep_store',
    titleKey: 'keepStoreDialogTitle',
    descKey: 'keepStoreDialogDesc',
    confirmKey: 'btnKeepStore',
    pendingKey: 'btnKeepingStore',
    errorKey: 'errKeepStoreFailed',
    icon: Shield,
    confirmVariant: 'primary',
  },
  {
    action: 'accept_source',
    titleKey: 'acceptSourceDialogTitle',
    descKey: 'acceptSourceDialogDesc',
    confirmKey: 'btnAcceptSource',
    pendingKey: 'btnAcceptingSource',
    errorKey: 'errAcceptSourceFailed',
    icon: ArrowDown,
    confirmVariant: 'destructive',
  },
  {
    action: 'rollback',
    titleKey: 'rollbackDialogTitle',
    descKey: 'rollbackDialogDesc',
    confirmKey: 'btnRollback',
    pendingKey: 'btnRollingBack',
    errorKey: 'errRollbackFailed',
    icon: ArrowBackUp,
    confirmVariant: 'primary',
  },
]

interface SyncActionsProps {
  skill: Skill
  dialog: SyncDialogName | null
  busy: SyncActionName | null
  dialogError: unknown | null
  onOpenDialog: (action: SyncDialogName) => void
  onCloseDialog: () => void
  onConfirm: (action: SyncActionName) => void
}

export function SyncActions({
  skill,
  dialog,
  busy,
  dialogError,
  onOpenDialog,
  onCloseDialog,
  onConfirm,
}: SyncActionsProps) {
  const { t, getErrorMessage } = useLocale()
  const actions = syncActionsFor(skill)
  const disabled = busy !== null

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={() => onConfirm('check')}>
          {busy === 'check' ? (
            <Spinner data-icon="start" currentColor className="text-[1.2em]" />
          ) : (
            <Refresh data-icon="start" />
          )}
          {busy === 'check' ? t('btnCheckingSync') : t('btnCheckSync')}
        </Button>
        {actions.includes('sync') && (
          <Button disabled={disabled} focusableWhenDisabled onClick={() => onConfirm('sync')}>
            {busy === 'sync' ? (
              <Spinner data-icon="start" currentColor className="text-[1.2em]" />
            ) : (
              <ArrowsExchange data-icon="start" />
            )}
            {busy === 'sync' ? t('btnSyncing') : t('btnSyncNow')}
          </Button>
        )}
        {actions.includes('keep_store') && (
          <Button variant="soft" disabled={disabled} focusableWhenDisabled onClick={() => onOpenDialog('keep_store')}>
            <Shield data-icon="start" />
            {t('btnKeepStore')}
          </Button>
        )}
        {actions.includes('accept_source') && (
          <Button
            variant="destructive"
            disabled={disabled}
            focusableWhenDisabled
            onClick={() => onOpenDialog('accept_source')}
          >
            <ArrowDown data-icon="start" />
            {t('btnAcceptSource')}
          </Button>
        )}
        {actions.includes('rollback') && (
          <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={() => onOpenDialog('rollback')}>
            <ArrowBackUp data-icon="start" />
            {t('btnRollback')}
          </Button>
        )}
      </div>

      {DIALOG_DEFS.map((def) => {
        const open = dialog === def.action
        const pending = busy === def.action
        const ConfirmIcon = def.icon
        return (
          <AlertDialog
            key={def.action}
            open={open}
            onOpenChange={(next) => {
              if (!next && !pending) onCloseDialog()
            }}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{t(def.titleKey)}</AlertDialogTitle>
                <AlertDialogDescription>{t(def.descKey, { name: skill.name })}</AlertDialogDescription>
              </AlertDialogHeader>

              {open && dialogError !== null && (
                <Alert variant="error" className="mt-4">
                  <AlertTitle>{t('alertSyncActionFailed')}</AlertTitle>
                  <AlertDescription>{getErrorMessage(dialogError, def.errorKey)}</AlertDescription>
                </Alert>
              )}

              <AlertDialogFooter>
                <Button variant="soft" disabled={pending} focusableWhenDisabled onClick={onCloseDialog}>
                  {t('btnCancel')}
                </Button>
                <Button
                  variant={def.confirmVariant}
                  disabled={pending}
                  focusableWhenDisabled
                  onClick={() => onConfirm(def.action)}
                >
                  {pending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
                  {pending ? t(def.pendingKey) : t(def.confirmKey)}
                  {!pending && <ConfirmIcon data-icon="end" />}
                </Button>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        )
      })}
    </>
  )
}
