import { useEffect, useMemo, useState, type ChangeEvent, type MouseEvent } from 'react'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Button } from '@appica/ui-react/button'
import { Checkbox } from '@appica/ui-react/checkbox'
import { Input } from '@appica/ui-react/input'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import {
  Pagination,
  PaginationList,
  PaginationItem,
  PaginationLink,
  PaginationEllipsis,
} from '@appica/ui-react/pagination'
import { Plus, Search, Refresh, Link as LinkIcon, Unlink, ChevronLeft, ChevronRight } from '@appica/icons-react'
import {
  inspectTarget,
  linkTargetSkill,
  unlinkTargetSkill,
  adoptTargetLink,
  type DistributionStatus,
} from './distribution-api'
import type { GroupSummary } from './group-api'
import type { Skill } from './skill-api'
import type { DesiredSkill } from './target-api'
import { useLocale } from './locale-context'
import { useNotifySuccess } from './notify-success'
import { paginationItems } from './pagination-items'
import { buildSkillRows, type SkillRow } from './target-skills-model'
import { TargetSkillRow, type RowAction } from './target-skill-row'
import { TargetSkillsAdd } from './target-skills-add'

const PAGE_SIZE = 10

function isAbort(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

interface TargetSkillsSectionProps {
  targetId: number
  desiredSkills: DesiredSkill[]
  availableSkills: Skill[]
  availableGroups: GroupSummary[]
  initialStatus: DistributionStatus | null
  submitting: boolean
  onAddAssignment: (kind: 'skill' | 'group', subjectIds: number[]) => Promise<DesiredSkill[]>
  onRemoveAssignments: (assignmentIds: number[]) => Promise<void>
}

export function TargetSkillsSection({
  targetId,
  desiredSkills,
  availableSkills,
  availableGroups,
  initialStatus,
  submitting,
  onAddAssignment,
  onRemoveAssignments,
}: TargetSkillsSectionProps) {
  const { t, getErrorMessage } = useLocale()
  const notify = useNotifySuccess()
  const [status, setStatus] = useState<DistributionStatus | null>(initialStatus)
  const [inspecting, setInspecting] = useState(false)
  const [actionError, setActionError] = useState<unknown | null>(null)
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [rowBusy, setRowBusy] = useState<{ id: number; action: RowAction } | null>(null)
  const [bulkBusy, setBulkBusy] = useState(false)
  const [removeRow, setRemoveRow] = useState<SkillRow | null>(null)
  const [refreshTick, setRefreshTick] = useState(0)

  const desiredSig = desiredSkills
    .map((d) => d.id)
    .sort((a, b) => a - b)
    .join(',')

  // Refresh Managed Link state on mount and whenever the desired set changes
  // (an Assignment was added or removed). Link/unlink update status inline.
  useEffect(() => {
    const controller = new AbortController()
    setInspecting(true)
    inspectTarget(targetId, controller.signal)
      .then(setStatus)
      .catch((err: unknown) => {
        if (!isAbort(err)) setActionError(err)
      })
      .finally(() => setInspecting(false))
    return () => controller.abort()
  }, [targetId, desiredSig, refreshTick])

  const rows = useMemo(() => buildSkillRows(desiredSkills, status), [desiredSkills, status])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return rows
    return rows.filter((r) => r.name.toLowerCase().includes(needle) || r.slug.toLowerCase().includes(needle))
  }, [rows, query])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const currentPage = Math.min(page, totalPages)
  const visible = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)
  const linkedVisible = visible.filter((r) => r.linked)
  const unlinkedVisible = visible.filter((r) => !r.linked)

  const busy = inspecting || bulkBusy || rowBusy !== null || submitting

  const onQueryChange = (e: ChangeEvent<HTMLInputElement>) => {
    setQuery(e.target.value)
    setPage(1)
  }
  const goToPage = (target: number) => (e: MouseEvent) => {
    e.preventDefault()
    setPage(Math.min(Math.max(target, 1), totalPages))
  }
  const toggleSelect = (id: number, checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  const selectAllVisible = (checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      for (const r of visible) {
        if (checked) next.add(r.id)
        else next.delete(r.id)
      }
      return next
    })

  const runRow = async (row: SkillRow, action: RowAction, op: () => Promise<DistributionStatus | void>) => {
    setRowBusy({ id: row.id, action })
    setActionError(null)
    try {
      const st = await op()
      if (st) setStatus(st)
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setRowBusy(null)
    }
  }

  const handleLink = (row: SkillRow) => runRow(row, 'link', () => linkTargetSkill(targetId, row.id))
  const handleUnlink = (row: SkillRow) => runRow(row, 'unlink', () => unlinkTargetSkill(targetId, row.id))
  const handleAdopt = (row: SkillRow) =>
    runRow(row, 'adopt', async () => {
      await adoptTargetLink(targetId, row.id)
      return inspectTarget(targetId)
    })

  const handleRemoveClick = (row: SkillRow) => {
    if (row.groups.length > 0) {
      setRemoveRow(row)
      return
    }
    runRow(row, 'remove', async () => {
      await onRemoveAssignments(row.assignmentIds)
    })
  }
  const confirmRemove = async () => {
    if (!removeRow) return
    const row = removeRow
    setRemoveRow(null)
    await runRow(row, 'remove', async () => {
      await onRemoveAssignments(row.assignmentIds)
    })
  }

  const handleAdd = async (kind: 'skill' | 'group', ids: number[]) => {
    setActionError(null)
    const present = new Set(rows.map((r) => r.id))
    try {
      if (kind === 'skill') {
        // Skill IDs are known here, so duplicates are caught before any POST.
        const fresh = ids.filter((id) => !present.has(id))
        if (fresh.length === 0) {
          notify(t('toastAlreadyAdded'), { description: t('toastAlreadyAddedSkill') })
          return
        }
        await onAddAssignment('skill', fresh)
      } else {
        // A Group's members aren't known until it is expanded, so add it and
        // detect from the new desired set whether it contributed any Skill.
        const updated = await onAddAssignment('group', ids)
        const added = updated.filter((d) => !present.has(d.id)).length
        if (added === 0) {
          notify(t('toastAlreadyAdded'), { description: t('toastAlreadyAddedGroup') })
        }
      }
      setPage(1)
    } catch (err: unknown) {
      setActionError(err)
    }
  }

  const runBulk = async (action: 'link' | 'unlink') => {
    const targets = rows.filter(
      (r) =>
        selected.has(r.id) &&
        (action === 'link' ? !r.linked && r.observed !== 'conflict' && r.observed !== 'broken_link' : r.linked),
    )
    if (targets.length === 0) return
    setBulkBusy(true)
    setActionError(null)
    let done = 0
    try {
      for (const row of targets) {
        const st = action === 'link' ? await linkTargetSkill(targetId, row.id) : await unlinkTargetSkill(targetId, row.id)
        setStatus(st)
        done++
      }
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setBulkBusy(false)
      setSelected(new Set())
      if (done > 0) notify(action === 'link' ? t('toastLinked') : t('toastUnlinked'), { description: t('toastLinkCount', { count: done }) })
    }
  }

  const allVisibleSelected = visible.length > 0 && visible.every((r) => selected.has(r.id))
  const someVisibleSelected = visible.some((r) => selected.has(r.id))
  const pageItems = paginationItems(currentPage, totalPages)

  const renderRow = (row: SkillRow) => (
    <TargetSkillRow
      key={row.id}
      row={row}
      selected={selected.has(row.id)}
      disabled={busy}
      busyAction={rowBusy?.id === row.id ? rowBusy.action : null}
      onSelect={(c) => toggleSelect(row.id, c)}
      onLink={() => handleLink(row)}
      onUnlink={() => handleUnlink(row)}
      onAdopt={() => handleAdopt(row)}
      onRemove={() => handleRemoveClick(row)}
    />
  )

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold flex flex-wrap items-center gap-2">
            <Plus className="size-5 text-foreground-muted" />
            {t('directAssignmentsHeading', { count: rows.length })}
          </h2>
        </div>
        <TargetSkillsAdd
          availableSkills={availableSkills}
          availableGroups={availableGroups}
          submitting={submitting}
          onAdd={handleAdd}
        />
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Input
          className="w-full max-w-xs"
          value={query}
          onChange={onQueryChange}
          onClear={() => {
            setQuery('')
            setPage(1)
          }}
          clearable
          startSlot={<Search />}
          placeholder={t('phSearchTargetSkills')}
          aria-label={t('ariaSearchTargetSkills')}
        />
        <span className="flex-1" />
        {selected.size > 0 && (
          <>
            <span className="text-sm text-foreground-muted">{t('selectedCount', { count: selected.size })}</span>
            <Button size="sm" variant="soft" disabled={busy} focusableWhenDisabled onClick={() => runBulk('link')}>
              <LinkIcon data-icon="start" />
              {t('btnLink')}
            </Button>
            <Button size="sm" variant="outline" disabled={busy} focusableWhenDisabled onClick={() => runBulk('unlink')}>
              <Unlink data-icon="start" />
              {t('btnUnlink')}
            </Button>
          </>
        )}
        <Button size="sm" variant="ghost" disabled={busy} focusableWhenDisabled onClick={() => setRefreshTick((x) => x + 1)} aria-label={t('btnInspect')}>
          {inspecting ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Refresh data-icon="start" />}
          {inspecting ? t('btnInspecting') : t('btnInspect')}
        </Button>
      </div>

      {actionError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertSkillActionFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(actionError, 'errInspectingTargetFailed')}</AlertDescription>
        </Alert>
      )}
      {status?.stale && (
        <Alert variant="warning">
          <AlertTitle>{t('distributionStale')}</AlertTitle>
          <AlertDescription>{t('staleObservation', { error: status.inspection_error || status.state_error || '' })}</AlertDescription>
        </Alert>
      )}

      {rows.length === 0 ? (
        <p className="text-foreground-muted text-sm py-6 text-center">{t('emptyTargetSkills')}</p>
      ) : filtered.length === 0 ? (
        <p role="status" className="text-foreground-muted text-sm py-6 text-center">
          {t('emptyTargetSkillsSearch')}
        </p>
      ) : (
        <>
          <div className="flex items-center gap-2 text-sm text-foreground-muted">
            <Checkbox
              checked={allVisibleSelected}
              indeterminate={someVisibleSelected && !allVisibleSelected}
              disabled={busy}
              onCheckedChange={selectAllVisible}
              aria-label={t('ariaSelectAllPage')}
            />
            <span>{t('ariaSelectAllPage')}</span>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <section className="space-y-2">
              <h3 className="text-sm font-semibold text-foreground-muted flex items-center gap-2">
                <LinkIcon className="size-4" />
                {t('paneLinkedHeading', { count: linkedVisible.length })}
              </h3>
              {linkedVisible.length === 0 ? (
                <p className="text-xs text-foreground-muted py-2">{t('paneEmptyLinked')}</p>
              ) : (
                linkedVisible.map(renderRow)
              )}
            </section>
            <section className="space-y-2">
              <h3 className="text-sm font-semibold text-foreground-muted flex items-center gap-2">
                <Unlink className="size-4" />
                {t('paneUnlinkedHeading', { count: unlinkedVisible.length })}
              </h3>
              {unlinkedVisible.length === 0 ? (
                <p className="text-xs text-foreground-muted py-2">{t('paneEmptyUnlinked')}</p>
              ) : (
                unlinkedVisible.map(renderRow)
              )}
            </section>
          </div>

          {totalPages > 1 && (
            <div className="max-w-full overflow-x-auto">
              <Pagination aria-label={t('ariaPagination')}>
                <PaginationList>
                  <PaginationItem>
                    <PaginationLink href="#!" aria-label={t('ariaPreviousPage')} className="px-0" disabled={currentPage === 1} onClick={goToPage(currentPage - 1)}>
                      <ChevronLeft />
                    </PaginationLink>
                  </PaginationItem>
                  {pageItems.map((item, index) =>
                    item === 'gap' ? (
                      <PaginationItem key={`gap-${index}`}>
                        <PaginationEllipsis />
                      </PaginationItem>
                    ) : item === currentPage ? (
                      <PaginationItem key={`page-${item}`}>
                        <PaginationLink active tabIndex={-1}>
                          {item}
                        </PaginationLink>
                      </PaginationItem>
                    ) : (
                      <PaginationItem key={`page-${item}`}>
                        <PaginationLink href="#!" aria-label={t('ariaGoToPage', { page: item })} onClick={goToPage(item)}>
                          {item}
                        </PaginationLink>
                      </PaginationItem>
                    ),
                  )}
                  <PaginationItem>
                    <PaginationLink href="#!" aria-label={t('ariaNextPage')} className="px-0" disabled={currentPage === totalPages} onClick={goToPage(currentPage + 1)}>
                      <ChevronRight />
                    </PaginationLink>
                  </PaginationItem>
                </PaginationList>
              </Pagination>
            </div>
          )}
        </>
      )}

      <AlertDialog open={removeRow !== null} onOpenChange={(open) => !open && setRemoveRow(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('removeGroupDialogTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('removeGroupDialogDesc', { name: removeRow?.name ?? '', groups: removeRow?.groups.join('、') ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <Button variant="outline" disabled={busy} focusableWhenDisabled onClick={() => setRemoveRow(null)}>
              {t('btnCancel')}
            </Button>
            <Button
              variant="primary"
              disabled={busy}
              focusableWhenDisabled
              onClick={confirmRemove}
              className="bg-error-emphasis hover:bg-error-emphasis"
            >
              {t('btnConfirmRemoveGroup')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
