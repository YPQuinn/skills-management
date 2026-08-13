import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import type { ImportItemResult } from './skill-api'
import { useLocale } from './locale-context'

interface SourceReplaceDialogProps {
  pendingConflict: ImportItemResult | null
  replacePending: boolean
  replaceError: unknown | null
  onConfirm: () => void
  onDecline: () => void
}

export function SourceReplaceDialog({
  pendingConflict,
  replacePending,
  replaceError,
  onConfirm,
  onDecline,
}: SourceReplaceDialogProps) {
  const { t, getErrorMessage } = useLocale()
  const open = pendingConflict !== null

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !replacePending) onDecline()
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('replaceDialogTitle')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('replaceDialogDesc', {
              name: pendingConflict?.replaces?.name || pendingConflict?.replaces?.slug || '',
              slug: pendingConflict?.replaces?.slug || '',
              dir: pendingConflict?.relative_dir || '',
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {replaceError !== null && (
          <Alert variant="error" className="mt-4">
            <AlertTitle>{t('alertReplaceFailed')}</AlertTitle>
            <AlertDescription>{getErrorMessage(replaceError, 'errImportingSkillsFailed')}</AlertDescription>
          </Alert>
        )}

        <AlertDialogFooter>
          <Button variant="soft" disabled={replacePending} onClick={onDecline}>
            {t('btnCancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={replacePending}
            onClick={onConfirm}
            focusableWhenDisabled
          >
            {replacePending && <Spinner data-icon="start" currentColor />}
            {replacePending ? t('btnReplacing') : t('btnReplaceSkill')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
