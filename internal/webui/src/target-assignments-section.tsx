import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxList,
  ComboboxItem,
} from '@appica/ui-react/combobox'
import { Plus, Trash, Folder, Sparkles } from '@appica/icons-react'
import type { Assignment } from './target-api'
import type { Skill } from './skill-api'
import type { GroupSummary } from './group-api'
import { useLocale } from './locale-context'
import { skillLabel } from './skill-identity'
import { RelativeTime } from './relative-time'

function AssignmentCombobox<T extends { id: number }>({
  items,
  selectedId,
  onSelectId,
  labelFor,
  placeholder,
  ariaLabel,
}: {
  items: T[]
  selectedId: string
  onSelectId: (id: string) => void
  labelFor: (item: T) => string
  placeholder: string
  ariaLabel: string
}) {
  const { t } = useLocale()
  const selected = items.find((item) => String(item.id) === selectedId) ?? null
  return (
    <Combobox
      items={items}
      value={selected}
      onValueChange={(v) => onSelectId(v ? String((v as T).id) : '')}
      itemToStringLabel={(item) => labelFor(item as T)}
      itemToStringValue={(item) => String((item as T).id)}
      autoHighlight
    >
      <ComboboxInput className="w-56" placeholder={placeholder} aria-label={ariaLabel} />
      <ComboboxContent>
        <ComboboxEmpty>{t('emptyCombobox')}</ComboboxEmpty>
        <ComboboxList>
          {(item: T) => (
            <ComboboxItem key={item.id} value={item}>
              {labelFor(item)}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}

interface TargetAssignmentsSectionProps {
  directSkills: Assignment[]
  groupAssignments: Assignment[]
  availableSkills: Skill[]
  availableGroups: GroupSummary[]
  submitting: boolean
  onAddAssignment: (kind: 'skill' | 'group', subjectId: number) => void
  onDeleteAssignment: (assignmentId: number) => void
}

export function TargetAssignmentsSection({
  directSkills,
  groupAssignments,
  availableSkills,
  availableGroups,
  submitting,
  onAddAssignment,
  onDeleteAssignment,
}: TargetAssignmentsSectionProps) {
  const { t } = useLocale()
  const [assignKind, setAssignKind] = useState<'skill' | 'group'>('skill')
  const [selectedSubjectId, setSelectedSubjectId] = useState<string>('')

  const allAssignments: Assignment[] = [...directSkills, ...groupAssignments]

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedSubjectId) return
    const idNum = Number(selectedSubjectId)
    if (Number.isNaN(idNum)) return
    onAddAssignment(assignKind, idNum)
    setSelectedSubjectId('')
  }

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Plus className="size-5 text-foreground-muted" />
          {t('directAssignmentsHeading', { count: allAssignments.length })}
        </h2>

        <form onSubmit={handleSubmit} className="flex flex-wrap items-center gap-2">
          <Select
            value={assignKind}
            onValueChange={(v) => {
              setAssignKind(v as 'skill' | 'group')
              setSelectedSubjectId('')
            }}
            items={{ skill: t('optAssignSkill'), group: t('optAssignGroup') }}
          >
            <SelectTrigger className="w-36" aria-label={t('labelAssignmentKind')}>
              <SelectValue placeholder={t('optAssignSkill')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="skill">{t('optAssignSkill')}</SelectItem>
              <SelectItem value="group">{t('optAssignGroup')}</SelectItem>
            </SelectContent>
          </Select>

          {assignKind === 'skill' ? (
            <AssignmentCombobox
              items={availableSkills}
              selectedId={selectedSubjectId}
              onSelectId={setSelectedSubjectId}
              labelFor={(s) => skillLabel(s.name, s.slug)}
              placeholder={t('phSelectSkill')}
              ariaLabel={t('optAssignSkill')}
            />
          ) : (
            <AssignmentCombobox
              items={availableGroups}
              selectedId={selectedSubjectId}
              onSelectId={setSelectedSubjectId}
              labelFor={(g) => g.name}
              placeholder={t('phSelectGroup')}
              ariaLabel={t('optAssignGroup')}
            />
          )}

          <Button type="submit" disabled={submitting || !selectedSubjectId} size="sm" focusableWhenDisabled>
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
                        <Badge variant="soft" className="capitalize">
                          {isSkill ? (
                            <Sparkles className="size-3 mr-1 inline" />
                          ) : (
                            <Folder className="size-3 mr-1 inline" />
                          )}
                          {as.kind}
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
                          className="text-error hover:text-error"
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
