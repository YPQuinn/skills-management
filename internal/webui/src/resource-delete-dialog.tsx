import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogBody,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import { useLocale } from './locale-context'

interface ResourceDeleteDialogProps {
  open: boolean
  title: string
  description: string
  details: string[]
  loading: boolean
  ready: boolean
  pending: boolean
  error: unknown | null
  warning?: string
  onConfirm: () => void
  onClose: () => void
}

export function ResourceDeleteDialog({
  open,
  title,
  description,
  details,
  loading,
  ready,
  pending,
  error,
  warning,
  onConfirm,
  onClose,
}: ResourceDeleteDialogProps) {
  const { t, getErrorMessage } = useLocale()
  const canConfirm = ready && !loading && !pending && !warning
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        // A warning result must stay visible until the user clicks Close.
        // Hiding the confirm button can emit onOpenChange(false); ignore that.
        if (!next && !pending && !warning) onClose()
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogBody className="space-y-3">
          {loading && (
            <div className="flex items-center gap-2 text-sm text-foreground-muted">
              <Spinner className="text-xl" aria-label={t('previewLoading')} />
              {t('previewLoading')}
            </div>
          )}
          {details.length > 0 && (
            <ul className="space-y-1 text-sm text-foreground-muted">
              {details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          )}
          {warning && (
            <Alert variant="warning">
              <AlertTitle>{t('alertOwnershipLost')}</AlertTitle>
              <AlertDescription>{warning}</AlertDescription>
            </Alert>
          )}
          {error !== null && (
            <Alert variant="error">
              <AlertTitle>{t('alertCleanupFailed')}</AlertTitle>
              <AlertDescription>{getErrorMessage(error, 'errCleanupFailed')}</AlertDescription>
            </Alert>
          )}
        </AlertDialogBody>
        <AlertDialogFooter>
          <Button variant="soft" disabled={pending} onClick={onClose}>
            {warning ? t('btnClose') : t('btnCancel')}
          </Button>
          {!warning && (
            <Button variant="destructive" disabled={!canConfirm} onClick={onConfirm} focusableWhenDisabled>
              {pending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
              {pending ? t('btnDeleting') : t('btnDelete')}
            </Button>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
