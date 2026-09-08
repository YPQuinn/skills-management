import { Chip } from '@appica/ui-react/chip'
import { countSyncFilters, SYNC_FILTERS, syncFilterLabelKey, type SyncFilter } from './sync-filter'
import type { Skill } from './skill-api'
import { useLocale } from './locale-context'

export function SyncFilterChips({
  skills,
  value,
  onChange,
}: {
  skills: Skill[]
  value: SyncFilter
  onChange: (next: SyncFilter) => void
}) {
  const { t } = useLocale()
  const counts = countSyncFilters(skills)

  return (
    <div className="flex flex-wrap gap-2" role="group" aria-label={t('ariaFilterSync')}>
      {SYNC_FILTERS.map((filter) => (
        <Chip
          key={filter}
          size="sm"
          variant={value === filter ? 'primary' : 'outline'}
          aria-pressed={value === filter}
          onClick={() => onChange(filter)}
        >
          {t(syncFilterLabelKey[filter], { count: counts[filter] })}
        </Chip>
      ))}
    </div>
  )
}
