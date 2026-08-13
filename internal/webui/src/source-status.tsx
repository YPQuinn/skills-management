import { Badge } from '@appica/ui-react/badge'
import { useLocale } from './locale-context'

export function SourceStatusBadge({ available, stale }: { available: boolean; stale: boolean }) {
  const { t } = useLocale()

  if (available) {
    return <Badge variant="success">{t('statusAvailable')}</Badge>
  }
  return (
    <>
      <Badge variant="error">{t('statusUnavailable')}</Badge>
      {stale && <Badge variant="warning">{t('statusStale')}</Badge>}
    </>
  )
}
