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

export function SyncStatusBadge({ status }: { status: string }) {
  const { t } = useLocale()
  if (!isSyncStatus(status)) {
    return <Badge variant="outline">{status}</Badge>
  }
  return <Badge variant={syncStatusVariant[status]}>{t(syncStatusKey[status])}</Badge>
}

export function SyncResultBadge({ result }: { result: string }) {
  const { t } = useLocale()
  if (!isSyncActionResult(result)) {
    return <Badge variant="outline">{result}</Badge>
  }
  return <Badge variant={syncResultVariant[result]}>{t(syncResultKey[result])}</Badge>
}
