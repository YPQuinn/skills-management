import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
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

        {pendingConflict?.impact && (
          <div className="mt-2 p-3 border border-border rounded-lg bg-background-subtle space-y-2 text-xs">
            <p className="font-semibold text-foreground-strong">{t('replaceImpactTitle')}</p>
            {pendingConflict.impact.groups && pendingConflict.impact.groups.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-foreground-subtle">{t('navGroups')}:</span>
                {pendingConflict.impact.groups.map((g) => (
                  <Badge key={g.id} variant="soft">
                    {g.name}
                  </Badge>
                ))}
              </div>
            )}
            {pendingConflict.impact.targets && pendingConflict.impact.targets.length > 0 && (
              <div className="space-y-1">
                <span className="text-foreground-subtle block">{t('navTargets')}:</span>
                <div className="flex flex-wrap items-center gap-1.5">
                  {pendingConflict.impact.targets.map((tgt) => {
                    const hasGroups = tgt.groups && tgt.groups.length > 0
                    return (
                      <div
                        key={tgt.id}
                        className="inline-flex items-center gap-1 p-1 px-2 border border-border rounded-md bg-background"
                      >
                        <span className="font-medium text-foreground-strong">{tgt.name}</span>
                        {tgt.direct && (
                          <Badge variant="soft" className="text-[10px] px-1 py-0 font-normal">
                            {t('reasonDirectTag')}
                          </Badge>
                        )}
                        {hasGroups &&
                          tgt.groups!.map((g) => (
                            <Badge key={g.id} variant="outline" className="text-[10px] px-1 py-0 font-normal">
                              {t('reasonViaGroupTag', { group: g.name })}
                            </Badge>
                          ))}
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        )}

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
            {replacePending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
            {replacePending ? t('btnReplacing') : t('btnReplaceSkill')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
