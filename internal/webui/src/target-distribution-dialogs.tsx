import { Button } from '@appica/ui-react/button'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import type { DistributionItemResult, DistributionItemView, DistributionResult } from './distribution-api'
import { useLocale } from './locale-context'
import { outcomeKey, resultKey } from './target-distribution-labels'

interface TargetDistributionDialogsProps {
  preview: DistributionResult | null
  result: DistributionResult | null
  confirmDistribute: boolean
  adoptItem: DistributionItemView | null
  disabled: boolean
  onClosePreview: () => void
  onCloseResult: () => void
  onConfirmOpenChange: (open: boolean) => void
  onCloseAdopt: () => void
  onConfirmDistribute: () => void
  onConfirmAdopt: () => void
}

function ItemResultList({ items }: { items: DistributionItemResult[] }) {
  const { t } = useLocale()
  return (
    <ul className="space-y-1 text-sm">
      {items.map((item) => (
        <li key={item.skill_id} className="font-mono">
          {item.slug}: {t(resultKey(item.result))}
          {item.error ? ` (${item.error})` : ''}
        </li>
      ))}
    </ul>
  )
}

export function TargetDistributionDialogs({
  preview,
  result,
  confirmDistribute,
  adoptItem,
  disabled,
  onClosePreview,
  onCloseResult,
  onConfirmOpenChange,
  onCloseAdopt,
  onConfirmDistribute,
  onConfirmAdopt,
}: TargetDistributionDialogsProps) {
  const { t } = useLocale()
  const blocked = result ? result.summary.blocked_conflict + result.summary.blocked_broken : 0
  return (
    <>
      <AlertDialog open={preview !== null} onOpenChange={(open) => !open && onClosePreview()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('previewTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('previewOutcome', { outcome: t(outcomeKey(preview?.outcome)) })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ItemResultList items={preview?.items ?? []} />
          <AlertDialogFooter>
            <Button variant="outline" onClick={onClosePreview}>
              {t('btnCancel')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmDistribute} onOpenChange={onConfirmOpenChange}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('distributeDialogTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('distributeDialogDesc')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={() => onConfirmOpenChange(false)}>
              {t('btnCancel')}
            </Button>
            <Button disabled={disabled} focusableWhenDisabled onClick={onConfirmDistribute}>
              {t('btnConfirmDistribute')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={result !== null} onOpenChange={(open) => !open && onCloseResult()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('distributionResultTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('distributionResultOutcome', { outcome: t(outcomeKey(result?.outcome)) })}
              {result && !result.dry_run && (
                <span className="block text-xs mt-1">
                  {result.summary.created > 0 && ` ${t('summaryCreated', { count: result.summary.created })}`}
                  {result.summary.removed > 0 && ` ${t('summaryRemoved', { count: result.summary.removed })}`}
                  {blocked > 0 && ` ${t('summaryBlocked', { count: blocked })}`}
                  {result.summary.failed > 0 && ` ${t('summaryFailed', { count: result.summary.failed })}`}
                </span>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ItemResultList items={result?.items ?? []} />
          <AlertDialogFooter>
            <Button variant="outline" onClick={onCloseResult}>
              {t('btnCancel')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={adoptItem !== null} onOpenChange={(open) => !open && onCloseAdopt()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('adoptDialogTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('adoptDialogDesc', { slug: adoptItem?.slug ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <Button variant="outline" disabled={disabled} focusableWhenDisabled onClick={onCloseAdopt}>
              {t('btnCancel')}
            </Button>
            <Button disabled={disabled} focusableWhenDisabled onClick={onConfirmAdopt}>
              {t('btnConfirmAdopt')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
