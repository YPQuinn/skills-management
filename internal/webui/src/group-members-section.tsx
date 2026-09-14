import { useState } from 'react'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Trash, UserPlus } from '@appica/icons-react'
import type { SkillRef } from './group-api'
import type { Skill } from './skill-api'
import { SkillCardIdentity, skillLabel } from './skill-identity'
import { useLocale } from './locale-context'

interface GroupMembersSectionProps {
  members: SkillRef[]
  skills: Skill[]
  submitting: boolean
  onAddMember: (skillIds: number[]) => void
  onRemoveMember: (skillId: number) => void
}

export function GroupMembersSection({
  members,
  skills,
  submitting,
  onAddMember,
  onRemoveMember,
}: GroupMembersSectionProps) {
  const { t } = useLocale()
  const [selectedSkillIds, setSelectedSkillIds] = useState<string[]>([])
  const memberIds = new Set(members.map((member) => member.id))
  const availableSkills = skills.filter((skill) => !memberIds.has(skill.id))
  const sourceById = new Map(
    skills.flatMap((skill) =>
      skill.binding?.source_name ? [[skill.id, skill.binding.source_name] as const] : [],
    ),
  )

  const skillLabels = Object.fromEntries(
    availableSkills.map((skill) => [String(skill.id), skillLabel(skill.name, skill.slug)]),
  )

  const handleAddSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const ids = selectedSkillIds.map(Number).filter((id) => !Number.isNaN(id))
    if (ids.length === 0) return
    onAddMember(ids)
    setSelectedSkillIds([])
  }

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <UserPlus className="size-5 text-foreground-muted" />
          {t('groupMembersHeading', { count: members.length })}
        </h2>

        {availableSkills.length > 0 && (
          <form onSubmit={handleAddSubmit} className="flex items-center gap-2">
            <Select
              multiple
              alignItemWithTrigger={false}
              value={selectedSkillIds}
              onValueChange={(val) => setSelectedSkillIds(val as string[])}
              items={skillLabels}
            >
              <SelectTrigger className="w-56" aria-label={t('selectSkillToAdd')}>
                <SelectValue placeholder={t('phSelectSkill')}>
                  {(selected: string[]) =>
                    selected.length === 0
                      ? t('phSelectSkill')
                      : selected.length === 1
                        ? (skillLabels[selected[0]] ?? selected[0])
                        : t('selectedCount', { count: selected.length })
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {availableSkills.map((skill) => (
                  <SelectItem key={skill.id} value={String(skill.id)}>
                    {skillLabel(skill.name, skill.slug)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button type="submit" disabled={submitting || selectedSkillIds.length === 0} size="sm">
              {submitting && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
              {submitting ? t('btnAddingMember') : t('btnAddMember')}
            </Button>
          </form>
        )}
      </div>

      {members.length === 0 ? (
        <p className="text-foreground-muted text-sm py-4">{t('emptyGroupMembers')}</p>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {members.map((member) => {
            const source = sourceById.get(member.id)
            return (
              <div
                key={member.id}
                className="flex items-center gap-2.5 rounded-lg border border-border px-3 py-2.5"
              >
                <SkillCardIdentity
                  name={member.name}
                  slug={member.slug}
                  sources={
                    source
                      ? [{ label: source, href: `/sources/${encodeURIComponent(source)}` }]
                      : undefined
                  }
                />
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={submitting}
                  aria-label={t('ariaRemoveMemberFor', { name: member.name })}
                  onClick={() => onRemoveMember(member.id)}
                  className="text-error-emphasis hover:text-error-emphasis"
                >
                  <Trash className="size-4" />
                </Button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
