// Sync Status and Sync Result as icon + text. Tone and icon sit here so
// the policy file stays free of React.
import {
  AlertCircle,
  AlertTriangle,
  Circle,
  CircleCheck,
  CircleDashed,
  CircleX,
  FileAlert,
  FileOff,
  GitCompare,
  RotateClockwise,
  Unlink,
} from '@appica/icons-react'
import { useLocale } from './locale-context'
import { isSyncActionResult, isSyncStatus, syncResultKey, syncStatusKey } from './sync-status-policy'
import { StatusLabel, type StatusIcon, type StatusSize, type StatusTone } from './status-label'
import type { SyncActionResult, SyncStatus } from './sync-api'

const syncStatusMark: Record<SyncStatus, { icon: StatusIcon; tone: StatusTone }> = {
  unbound: { icon: Unlink, tone: 'muted' },
  unchecked: { icon: CircleDashed, tone: 'muted' },
  in_sync: { icon: CircleCheck, tone: 'muted' },
  source_changed: { icon: GitCompare, tone: 'info' },
  store_changed: { icon: GitCompare, tone: 'info' },
  conflict: { icon: AlertTriangle, tone: 'error' },
  source_missing: { icon: FileOff, tone: 'warning' },
  source_invalid: { icon: FileAlert, tone: 'warning' },
  store_missing: { icon: FileOff, tone: 'error' },
  store_invalid: { icon: FileAlert, tone: 'error' },
}

const syncResultMark: Record<SyncActionResult, { icon: StatusIcon; tone: StatusTone }> = {
  no_op: { icon: Circle, tone: 'muted' },
  updated: { icon: CircleCheck, tone: 'muted' },
  kept_store: { icon: CircleCheck, tone: 'info' },
  accepted_source: { icon: CircleCheck, tone: 'muted' },
  skipped: { icon: AlertCircle, tone: 'warning' },
  blocked: { icon: AlertTriangle, tone: 'warning' },
  failed: { icon: CircleX, tone: 'error' },
  rolled_back: { icon: RotateClockwise, tone: 'info' },
}

const unknownMark = { icon: Circle, tone: 'muted' as const }

export function SyncStatusBadge({
  status,
  count,
  size,
}: {
  status: string
  count?: number
  size?: StatusSize
}) {
  const { t } = useLocale()
  const known = isSyncStatus(status)
  const label = known ? t(syncStatusKey[status]) : status
  const mark = known ? syncStatusMark[status] : unknownMark
  return (
    <StatusLabel icon={mark.icon} tone={mark.tone} size={size}>
      {count === undefined ? label : `${count} ${label}`}
    </StatusLabel>
  )
}

export function SyncResultBadge({
  result,
  count,
  size,
}: {
  result: string
  count?: number
  size?: StatusSize
}) {
  const { t } = useLocale()
  const known = isSyncActionResult(result)
  const label = known ? t(syncResultKey[result]) : result
  const mark = known ? syncResultMark[result] : unknownMark
  return (
    <StatusLabel icon={mark.icon} tone={mark.tone} size={size}>
      {count === undefined ? label : `${count} ${label}`}
    </StatusLabel>
  )
}
