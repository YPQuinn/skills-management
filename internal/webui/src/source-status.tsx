import { CircleCheck, CircleX, Clock } from '@appica/icons-react'
import { StatusLabel, type StatusSize } from './status-label'
import { useLocale } from './locale-context'

export function SourceStatusBadge({
  available,
  stale,
  size = 'sm',
}: {
  available: boolean
  stale: boolean
  size?: StatusSize
}) {
  const { t } = useLocale()

  if (available) {
    return (
      <StatusLabel icon={CircleCheck} tone="muted" size={size}>
        {t('statusAvailable')}
      </StatusLabel>
    )
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <StatusLabel icon={CircleX} tone="error" size={size}>
        {t('statusUnavailable')}
      </StatusLabel>
      {stale && (
        <StatusLabel icon={Clock} tone="warning" size={size}>
          {t('statusStale')}
        </StatusLabel>
      )}
    </span>
  )
}
