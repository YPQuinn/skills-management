import { Badge } from '@appica/ui-react/badge'
import { Button } from '@appica/ui-react/button'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@appica/ui-react/dialog'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import type { DistributionItemResult, DistributionItemView, DistributionResult } from './distribution-api'
import { useLocale, type LocaleContextType } from './locale-context'
import { itemResultBadge, outcomeBadgeVariant, outcomeKey } from './target-distribution-labels'

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

function ItemResultTable({ items, caption }: { items: DistributionItemResult[]; caption: string }) {
  const { t } = useLocale()
  if (items.length === 0) {
    return <p className="text-foreground-muted text-sm py-2">{t('emptyDistributionItems')}</p>
  }
  return (
    <Table size="sm" aria-label={caption}>
      <TableCaption className="sr-only">{caption}</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>{t('colName')}</TableHead>
          <TableHead>{t('colItemResult')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => {
          const badge = itemResultBadge(item.result)
          return (
            <TableRow key={item.skill_id}>
              <TableCell>{item.slug}</TableCell>
              <TableCell>
                <div className="flex flex-col items-start gap-1">
                  <Badge variant={badge.variant}>{t(badge.key)}</Badge>
                  {item.error ? <span className="text-xs text-foreground-muted">{item.error}</span> : null}
                </div>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function resultSummary(result: DistributionResult, t: LocaleContextType['t']): string {
  const blocked = result.summary.blocked_conflict + result.summary.blocked_broken
  return [
    result.summary.created > 0 ? t('summaryCreated', { count: result.summary.created }) : '',
    result.summary.removed > 0 ? t('summaryRemoved', { count: result.summary.removed }) : '',
    blocked > 0 ? t('summaryBlocked', { count: blocked }) : '',
    result.summary.failed > 0 ? t('summaryFailed', { count: result.summary.failed }) : '',
  ]
    .filter(Boolean)
    .join(' · ')
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
  const summary = result && !result.dry_run ? resultSummary(result, t) : ''
  return (
    <>
      <Dialog open={preview !== null} onOpenChange={(open) => !open && onClosePreview()}>
        <DialogContent className="sm:max-w-lg" closeLabel={t('btnClose')}>
          <DialogHeader>
            <div className="flex flex-wrap items-center gap-2">
              <DialogTitle>{t('previewTitle')}</DialogTitle>
              {preview && (
                <Badge variant={outcomeBadgeVariant(preview.outcome)}>{t(outcomeKey(preview.outcome))}</Badge>
              )}
            </div>
            <DialogDescription className="sr-only">
              {t('previewOutcome', { outcome: t(outcomeKey(preview?.outcome)) })}
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <ItemResultTable items={preview?.items ?? []} caption={t('previewTitle')} />
          </DialogBody>
        </DialogContent>
      </Dialog>

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

      <Dialog open={result !== null} onOpenChange={(open) => !open && onCloseResult()}>
        <DialogContent className="sm:max-w-lg" closeLabel={t('btnClose')}>
          <DialogHeader>
            <div className="flex flex-wrap items-center gap-2">
              <DialogTitle>{t('distributionResultTitle')}</DialogTitle>
              {result && (
                <Badge variant={outcomeBadgeVariant(result.outcome)}>{t(outcomeKey(result.outcome))}</Badge>
              )}
            </div>
            <DialogDescription className={summary ? undefined : 'sr-only'}>
              {summary || t('distributionResultOutcome', { outcome: t(outcomeKey(result?.outcome)) })}
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <ItemResultTable items={result?.items ?? []} caption={t('distributionResultTitle')} />
          </DialogBody>
        </DialogContent>
      </Dialog>

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
