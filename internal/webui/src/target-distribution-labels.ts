import type { DistributionItemView } from './distribution-api'
import type { DictionaryKey } from './locale-dictionary'

export function desiredBadge(item: DistributionItemView) {
  if (item.desired === 'present') return { key: 'desiredPresent' as const, variant: 'soft' as const }
  return { key: 'desiredAbsent' as const, variant: 'outline' as const }
}

export function observedBadge(item: DistributionItemView) {
  switch (item.observed) {
    case 'linked':
      return { key: 'observedLinked' as const, variant: 'success' as const }
    case 'missing':
      return { key: 'observedMissing' as const, variant: 'outline' as const }
    case 'conflict':
      return { key: 'observedConflict' as const, variant: 'error' as const }
    case 'broken_link':
      return { key: 'observedBrokenLink' as const, variant: 'warning' as const }
  }
}

export function stateKey(state: string): DictionaryKey {
  switch (state) {
    case 'ok':
      return 'distributionStateOk'
    case 'missing':
      return 'distributionStateMissing'
    case 'redirected':
      return 'distributionStateRedirected'
    case 'invalid':
      return 'distributionStateInvalid'
    default:
      return 'distributionStateStored'
  }
}

export function resultKey(result: string): DictionaryKey {
  switch (result) {
    case 'created':
      return 'itemResultCreated'
    case 'removed':
      return 'itemResultRemoved'
    case 'adopted':
      return 'itemResultAdopted'
    case 'blocked_conflict':
      return 'itemResultBlockedConflict'
    case 'blocked_broken':
      return 'itemResultBlockedBroken'
    case 'ownership_lost':
      return 'itemResultOwnershipLost'
    case 'failed':
      return 'itemResultFailed'
    default:
      return 'itemResultNoOp'
  }
}

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
