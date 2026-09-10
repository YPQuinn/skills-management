import type { DictionaryKey } from './locale-dictionary'
import { AlertTriangle, CircleCheck, CircleX } from '@appica/icons-react'
import type { StatusIcon, StatusTone } from './status-label'

export function outcomeKey(outcome: string | undefined): DictionaryKey {
  switch (outcome) {
    case 'partial':
      return 'outcomePartial'
    case 'blocked':
      return 'outcomeBlocked'
    case 'failed':
      return 'outcomeFailed'
    default:
      return 'outcomeSucceeded'
  }
}

export function outcomeMark(outcome: string): { icon: StatusIcon; tone: StatusTone } {
  if (outcome === 'succeeded') return { icon: CircleCheck, tone: 'muted' }
  if (outcome === 'failed') return { icon: CircleX, tone: 'error' }
  return { icon: AlertTriangle, tone: 'warning' }
}
