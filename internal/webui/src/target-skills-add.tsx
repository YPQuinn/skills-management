import { useState } from 'react'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Plus } from '@appica/icons-react'
import type { Skill } from './skill-api'
import type { GroupSummary } from './group-api'
import { useLocale } from './locale-context'
import { skillLabel } from './skill-identity'

interface TargetSkillsAddProps {
  availableSkills: Skill[]
  availableGroups: GroupSummary[]
  submitting: boolean
  onAdd: (kind: 'skill' | 'group', subjectIds: number[]) => Promise<void>
}

export function TargetSkillsAdd({ availableSkills, availableGroups, submitting, onAdd }: TargetSkillsAddProps) {
  const { t } = useLocale()
  const [kind, setKind] = useState<'skill' | 'group'>('skill')
  const [selectedIds, setSelectedIds] = useState<string[]>([])

  const items =
    kind === 'skill'
      ? Object.fromEntries(availableSkills.map((s) => [String(s.id), skillLabel(s.name, s.slug)]))
      : Object.fromEntries(availableGroups.map((g) => [String(g.id), g.name]))
  const empty = Object.keys(items).length === 0
  const idle = empty
    ? kind === 'skill'
      ? t('emptyAssignableSkills')
      : t('emptyAssignableGroups')
    : kind === 'skill'
      ? t('phSelectSkill')
      : t('phSelectGroup')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const ids = selectedIds.map(Number).filter((n) => !Number.isNaN(n))
    if (ids.length === 0) return
    await onAdd(kind, ids)
    setSelectedIds([])
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-wrap items-center gap-2">
      <Select
        value={kind}
        onValueChange={(v) => {
          setKind(v as 'skill' | 'group')
          setSelectedIds([])
        }}
        items={{ skill: t('optAssignSkill'), group: t('optAssignGroup') }}
      >
        <SelectTrigger className="w-32" aria-label={t('labelAssignmentKind')}>
          <SelectValue placeholder={t('optAssignSkill')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="skill">{t('optAssignSkill')}</SelectItem>
          <SelectItem value="group">{t('optAssignGroup')}</SelectItem>
        </SelectContent>
      </Select>

      <Select
        multiple
        disabled={empty}
        alignItemWithTrigger={false}
        value={selectedIds}
        onValueChange={(v) => setSelectedIds(v as string[])}
        items={items}
      >
        <SelectTrigger className="w-56" aria-label={kind === 'skill' ? t('optAssignSkill') : t('optAssignGroup')}>
          <SelectValue placeholder={idle}>
            {(sel: string[]) =>
              sel.length === 0
                ? idle
                : sel.length === 1
                  ? (items[sel[0]] ?? sel[0])
                  : t('selectedCount', { count: sel.length })
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {Object.entries(items).map(([id, label]) => (
            <SelectItem key={id} value={id}>
              {label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Button type="submit" size="sm" disabled={submitting || selectedIds.length === 0} focusableWhenDisabled>
        {submitting ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Plus data-icon="start" />}
        {t('btnAssign')}
      </Button>
    </form>
  )
}
