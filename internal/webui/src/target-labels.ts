import type { DictionaryKey } from './locale-dictionary'
import type { TargetAdapter } from './target-api'

type Translate = (key: DictionaryKey) => string

// Targets store the adapter and scope as raw keys. Both views render the same
// labels the create dialog offers, so the stored keys never reach the user.

export function adapterLabel(key: string, adapters: TargetAdapter[], t: Translate): string {
  const known = adapters.find((a) => a.key === key)
  if (known) return known.name
  return key === 'custom' ? t('optCustomTarget') : key
}

export function scopeLabel(scope: string, t: Translate): string {
  if (scope === 'user') return t('optScopeUser')
  if (scope === 'project') return t('optScopeProject')
  if (scope === 'custom') return t('optScopeCustom')
  return scope
}
