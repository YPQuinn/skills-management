import { Badge } from '@appica/ui-react/badge'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Check } from '@appica/icons-react'
import type { DistributionItemView } from './distribution-api'
import { useLocale } from './locale-context'
import { desiredBadge, observedBadge, resultKey } from './target-distribution-labels'

export type DistributionBusy = 'inspect' | 'preview' | 'distribute' | `adopt-${number}` | null

interface TargetDistributionTableProps {
  items: DistributionItemView[]
  busy: DistributionBusy
  disabled: boolean
  onAdopt: (item: DistributionItemView) => void
}

export function TargetDistributionTable({ items, busy, disabled, onAdopt }: TargetDistributionTableProps) {
  const { t } = useLocale()
  if (items.length === 0) {
    return <p className="text-foreground-muted text-sm py-4">{t('emptyDistributionItems')}</p>
  }
  return (
    <ScrollArea className="w-full" orientation="horizontal">
      <div className="min-w-[700px]">
        <Table aria-label={t('captionDistributionTable')}>
          <TableCaption className="sr-only">{t('captionDistributionTable')}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead>{t('colSlug')}</TableHead>
              <TableHead>{t('colDesired')}</TableHead>
              <TableHead>{t('colObserved')}</TableHead>
              <TableHead>{t('colLastResult')}</TableHead>
              <TableHead>{t('colDistributionActions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => {
              const desired = desiredBadge(item)
              const observed = observedBadge(item)
              return (
                <TableRow key={item.skill_id}>
                  <TableCell className="font-mono text-foreground-muted">{item.slug}</TableCell>
                  <TableCell>
                    <Badge variant={desired.variant}>{t(desired.key)}</Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant={observed.variant}>{t(observed.key)}</Badge>
                    {item.adoptable && (
                      <span className="ml-2 text-xs text-foreground-muted">(adoptable)</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {item.last_result ? t(resultKey(item.last_result)) : ''}
                  </TableCell>
                  <TableCell>
                    {item.adoptable && item.desired === 'present' && (
                      <Button
                        variant="soft"
                        size="sm"
                        disabled={disabled}
                        focusableWhenDisabled
                        onClick={() => onAdopt(item)}
                      >
                        {busy === `adopt-${item.skill_id}` ? (
                          <Spinner data-icon="start" currentColor className="text-[1.2em]" />
                        ) : (
                          <Check data-icon="start" />
                        )}
                        {busy === `adopt-${item.skill_id}` ? t('btnAdopting') : t('btnAdopt')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </ScrollArea>
  )
}
