import { Badge } from '@appica/ui-react/badge'
import { Button } from '@appica/ui-react/button'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Check, X, InfoCircle, Plus } from '@appica/icons-react'
import type { TargetAdapter } from './target-api'
import { useLocale } from './locale-context'

export function TargetAdapterTable({
  adapters,
  onSelectAdapter,
}: {
  adapters: TargetAdapter[]
  onSelectAdapter: (key: string) => void
}) {
  const { t } = useLocale()
  if (adapters.length === 0) return null
  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold">{t('adaptersTitle')}</h3>
      <ScrollArea className="w-full" orientation="horizontal">
        <div className="min-w-[650px]">
          <Table aria-label={t('captionAdaptersTable')}>
            <TableCaption className="sr-only">{t('captionAdaptersTable')}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>{t('colName')}</TableHead>
                <TableHead>{t('colAdapter')}</TableHead>
                <TableHead>{t('colDetectionStatus')}</TableHead>
                <TableHead>{t('colEvidence')}</TableHead>
                <TableHead className="text-end">{t('colActions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {adapters.map((ad) => {
                const status = ad.detection.status
                const isDetected = status === 'detected'
                const isNotDetected = status === 'not_detected' || status === 'not_found'
                const isNotApplicable = status === 'not_applicable'

                let statusLabel = t('statusUnknown')
                if (isDetected) statusLabel = t('statusDetected')
                else if (isNotDetected) statusLabel = t('statusNotDetected')
                else if (isNotApplicable) statusLabel = t('statusNotApplicable')
                else if (status === 'unknown') statusLabel = t('statusUnknown')

                return (
                  <TableRow key={ad.key}>
                    <TableCell className="font-medium text-foreground-strong">{ad.name}</TableCell>
                    <TableCell className="font-mono text-foreground-muted">{ad.key}</TableCell>
                    <TableCell>
                      <Badge variant={isDetected ? 'soft' : 'outline'} className="gap-1">
                        {isDetected ? (
                          <Check className="size-3 text-success inline" />
                        ) : isNotDetected ? (
                          <X className="size-3 text-foreground-muted inline" />
                        ) : (
                          <InfoCircle className="size-3 inline" />
                        )}
                        {statusLabel}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-foreground-muted text-xs">
                      {ad.detection.evidence && ad.detection.evidence.length > 0
                        ? ad.detection.evidence.join('; ')
                        : '—'}
                    </TableCell>
                    <TableCell className="text-end">
                      <Button variant="ghost" size="sm" onClick={() => onSelectAdapter(ad.key)}>
                        <Plus className="size-4" />
                        {t('btnRegisterAdapter')}
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </ScrollArea>
    </div>
  )
}
