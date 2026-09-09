import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Plus, Trash, Folder, Sparkles } from '@appica/icons-react'
import type { Assignment, DesiredSkill } from './target-api'
import type { Skill } from './skill-api'
import type { GroupSummary } from './group-api'
import { useLocale } from './locale-context'
import { skillLabel } from './skill-identity'
import { RelativeTime } from './relative-time'
import { TargetDesiredSetPreview } from './target-desired-set-section'

function AssignmentMultiSelect<T extends { id: number }>({
  items,
  selectedIds,
  onSelectedIds,
  labelFor,
  placeholder,
  emptyPlaceholder,
  ariaLabel,
}: {
  items: T[]
  selectedIds: string[]
  onSelectedIds: (ids: string[]) => void
  labelFor: (item: T) => string
  placeholder: string
  emptyPlaceholder: string
  ariaLabel: string
}) {
  const { t } = useLocale()
  const labels = Object.fromEntries(items.map((item) => [String(item.id), labelFor(item)]))
  const empty = items.length === 0
  const idleLabel = empty ? emptyPlaceholder : placeholder
  return (
    <Select
      multiple
      disabled={empty}
      alignItemWithTrigger={false}
      value={selectedIds}
      onValueChange={(v) => onSelectedIds(v as string[])}
      items={labels}
    >
      <SelectTrigger className="w-56" aria-label={ariaLabel}>
        <SelectValue placeholder={idleLabel}>
          {(selected: string[]) =>
            selected.length === 0
              ? idleLabel
              : selected.length === 1
                ? (labels[selected[0]] ?? selected[0])
                : t('selectedCount', { count: selected.length })
          }
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.id} value={String(item.id)}>
            {labelFor(item)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

interface TargetAssignmentsSectionProps {
  directSkills: Assignment[]
  groupAssignments: Assignment[]
  availableSkills: Skill[]
  availableGroups: GroupSummary[]
  desiredSkills: DesiredSkill[]
  submitting: boolean
  onAddAssignment: (kind: 'skill' | 'group', subjectIds: number[]) => void
  onDeleteAssignment: (assignmentId: number) => void
}

export function TargetAssignmentsSection({
  directSkills,
  groupAssignments,
  availableSkills,
  availableGroups,
  desiredSkills,
  submitting,
  onAddAssignment,
  onDeleteAssignment,
}: TargetAssignmentsSectionProps) {
  const { t } = useLocale()
  const [assignKind, setAssignKind] = useState<'skill' | 'group'>('skill')
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([])

  const allAssignments: Assignment[] = [...directSkills, ...groupAssignments]

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const ids = selectedSubjectIds.map(Number).filter((id) => !Number.isNaN(id))
    if (ids.length === 0) return
    onAddAssignment(assignKind, ids)
    setSelectedSubjectIds([])
  }

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <h2 className="text-lg font-semibold flex items-center gap-2">
            <Plus className="size-5 text-foreground-muted" />
            {t('directAssignmentsHeading', { count: allAssignments.length })}
          </h2>
          <TargetDesiredSetPreview desiredSkills={desiredSkills} />
        </div>

        <form onSubmit={handleSubmit} className="flex flex-wrap items-center gap-2">
          <Select
            value={assignKind}
            onValueChange={(v) => {
              setAssignKind(v as 'skill' | 'group')
              setSelectedSubjectIds([])
            }}
            items={{ skill: t('optAssignSkill'), group: t('optAssignGroup') }}
          >
            <SelectTrigger className="w-40" aria-label={t('labelAssignmentKind')}>
              <SelectValue placeholder={t('optAssignSkill')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="skill">{t('optAssignSkill')}</SelectItem>
              <SelectItem value="group">{t('optAssignGroup')}</SelectItem>
            </SelectContent>
          </Select>

          {assignKind === 'skill' ? (
            <AssignmentMultiSelect
              items={availableSkills}
              selectedIds={selectedSubjectIds}
              onSelectedIds={setSelectedSubjectIds}
              labelFor={(s) => skillLabel(s.name, s.slug)}
              placeholder={t('phSelectSkill')}
              emptyPlaceholder={t('emptyAssignableSkills')}
              ariaLabel={t('optAssignSkill')}
            />
          ) : (
            <AssignmentMultiSelect
              items={availableGroups}
              selectedIds={selectedSubjectIds}
              onSelectedIds={setSelectedSubjectIds}
              labelFor={(g) => g.name}
              placeholder={t('phSelectGroup')}
              emptyPlaceholder={t('emptyAssignableGroups')}
              ariaLabel={t('optAssignGroup')}
            />
          )}

          <Button type="submit" disabled={submitting || selectedSubjectIds.length === 0} size="sm" focusableWhenDisabled>
            {submitting && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
            {submitting ? t('btnAssigning') : t('btnAssign')}
          </Button>
        </form>
      </div>

      {allAssignments.length === 0 ? (
        <p className="text-foreground-muted text-sm py-4">{t('emptyDirectAssignments')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[600px]">
            <Table aria-label={t('directAssignmentsHeading', { count: allAssignments.length })}>
              <TableCaption className="sr-only">
                {t('directAssignmentsHeading', { count: allAssignments.length })}
              </TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('labelAssignmentKind')}</TableHead>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead>{t('labelCreated')}</TableHead>
                  <TableHead className="text-end">{t('colActions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {allAssignments.map((as) => {
                  const isSkill = as.kind === 'skill'
                  const name = isSkill ? as.skill?.name : as.group?.name
                  const link =
                    isSkill && as.skill
                      ? `/skills/${encodeURIComponent(as.skill.slug)}`
                      : !isSkill && as.group
                        ? `/groups/${encodeURIComponent(as.group.name)}`
                        : '#'

                  return (
                    <TableRow key={as.id}>
                      <TableCell>
                        <Badge variant="soft">
                          {isSkill ? (
                            <Sparkles className="size-3 mr-1 inline" />
                          ) : (
                            <Folder className="size-3 mr-1 inline" />
                          )}
                          {isSkill ? t('optAssignSkill') : t('optAssignGroup')}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-medium text-foreground-strong">
                        <Link
                          to={link}
                          className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                        >
                          {name}
                        </Link>
                      </TableCell>
                      <TableCell className="text-foreground-muted text-xs">
                        <RelativeTime value={as.created_at} />
                      </TableCell>
                      <TableCell className="text-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={submitting}
                          aria-label={t('ariaRemoveAssignmentFor', { name: name || String(as.id) })}
                          onClick={() => onDeleteAssignment(as.id)}
                          className="text-error-emphasis hover:text-error-emphasis"
                        >
                          <Trash className="size-4" />
                          {t('btnRemoveAssignment')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        </ScrollArea>
      )}
    </div>
  )
}
