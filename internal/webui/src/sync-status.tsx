// The Sync Status and Sync Result Appica badges: text labels with
// semantic variants resolved by the presentation policy in sync-status.ts.
import { Badge } from '@appica/ui-react/badge'
import { useLocale } from './locale-context'
import {
  isSyncActionResult,
  isSyncStatus,
  syncResultKey,
  syncResultVariant,
  syncStatusKey,
  syncStatusVariant,
} from './sync-status-policy'

type BadgeSize = 'xs' | 'sm' | 'md' | 'lg'

// count turns the badge into a tally ("2 in sync"); both dictionaries put
// the number first, as the Skills index filter chips already do.
export function SyncStatusBadge({
  status,
  count,
  size,
}: {
  status: string
  count?: number
  size?: BadgeSize
}) {
  const { t } = useLocale()
  const known = isSyncStatus(status)
  const label = known ? t(syncStatusKey[status]) : status
  return (
    <Badge variant={known ? syncStatusVariant[status] : 'outline'} size={size}>
      {count === undefined ? label : `${count} ${label}`}
    </Badge>
  )
}

export function SyncResultBadge({
  result,
  count,
  size,
}: {
  result: string
  count?: number
  size?: BadgeSize
}) {
  const { t } = useLocale()
  const known = isSyncActionResult(result)
  const label = known ? t(syncResultKey[result]) : result
  return (
    <Badge variant={known ? syncResultVariant[result] : 'outline'} size={size}>
      {count === undefined ? label : `${count} ${label}`}
    </Badge>
  )
}
