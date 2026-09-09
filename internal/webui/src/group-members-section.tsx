import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Trash, UserPlus } from '@appica/icons-react'
import type { SkillRef } from './group-api'
import type { Skill } from './skill-api'
import { useLocale } from './locale-context'

interface GroupMembersSectionProps {
  members: SkillRef[]
  availableSkills: Skill[]
  submitting: boolean
  onAddMember: (skillId: number) => void
  onRemoveMember: (skillId: number) => void
}

export function GroupMembersSection({
  members,
  availableSkills,
  submitting,
  onAddMember,
  onRemoveMember,
}: GroupMembersSectionProps) {
  const { t } = useLocale()
  const [selectedSkillId, setSelectedSkillId] = useState<string>('')

  const handleAddSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedSkillId) return
    const idNum = Number(selectedSkillId)
    if (Number.isNaN(idNum)) return
    onAddMember(idNum)
    setSelectedSkillId('')
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
              value={selectedSkillId}
              onValueChange={(val) => setSelectedSkillId(val as string)}
              items={Object.fromEntries(
                availableSkills.map((skill) => [String(skill.id), `${skill.name} (${skill.slug})`]),
              )}
            >
              <SelectTrigger className="w-56" aria-label={t('selectSkillToAdd')}>
                <SelectValue placeholder={t('phSelectSkill')} />
              </SelectTrigger>
              <SelectContent>
                {availableSkills.map((skill) => (
                  <SelectItem key={skill.id} value={String(skill.id)}>
                    {skill.name} ({skill.slug})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button type="submit" disabled={submitting || !selectedSkillId} size="sm">
              {submitting && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
              {submitting ? t('btnAddingMember') : t('btnAddMember')}
            </Button>
          </form>
        )}
      </div>

      {members.length === 0 ? (
        <p className="text-foreground-muted text-sm py-4">{t('emptyGroupMembers')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[600px]">
            <Table aria-label={t('groupMembersHeading', { count: members.length })}>
              <TableCaption className="sr-only">
                {t('groupMembersHeading', { count: members.length })}
              </TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead>{t('colSlug')}</TableHead>
                  <TableHead className="text-end">{t('colActions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((member) => (
                  <TableRow key={member.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link
                        to={`/skills/${encodeURIComponent(member.slug)}`}
                        className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                      >
                        {member.name}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-foreground-muted">{member.slug}</TableCell>
                    <TableCell className="text-end">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={submitting}
                        aria-label={t('ariaRemoveMemberFor', { name: member.name })}
                        onClick={() => onRemoveMember(member.id)}
                        className="text-error hover:text-error"
                      >
                        <Trash className="size-4" />
                        {t('btnRemoveMember')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </ScrollArea>
      )}
    </div>
  )
}
