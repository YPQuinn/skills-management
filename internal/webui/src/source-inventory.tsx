import { useMemo, useState, type ChangeEvent, type MouseEvent, type ReactNode } from 'react'
import { Button } from '@appica/ui-react/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Checkbox } from '@appica/ui-react/checkbox'
import { Input } from '@appica/ui-react/input'
import { Pagination, PaginationList, PaginationItem, PaginationLink, PaginationEllipsis } from '@appica/ui-react/pagination'
import { Spinner } from '@appica/ui-react/spinner'
import { ChevronLeft, ChevronRight, Refresh, Search } from '@appica/icons-react'
import { SourceInventoryTable } from './source-inventory-table'
import { paginationItems } from './pagination-items'
import { RelativeTime } from './relative-time'
import type { SourceEntry } from './source-api'
import type { Skill, ImportResponse } from './skill-api'
import { useLocale } from './locale-context'

const PAGE_SIZE = 10

interface SourceInventoryProps {
  inventory: SourceEntry[]
  available: boolean
  lastScannedAt?: string
  rescanning: boolean
  onRescan: () => void
  boundSkillMap: Map<string, Skill>
  selectedDirs: string[]
  setSelectedDirs: (dirs: string[]) => void
  slugOverrides: Record<string, string>
  onSlugOverrideChange: (dir: string, value: string) => void
  allowLarge: boolean
  setAllowLarge: (val: boolean) => void
  importing: boolean
  importResult: ImportResponse | null
  onImport: (options: { all?: boolean }) => void
  // Synchronization is a Source action, not an Inventory one, so the page
  // hands its button and its status in rather than this module owning them.
  syncAction?: ReactNode
  syncStatus?: ReactNode
}

export function SourceInventory({
  inventory,
  available,
  lastScannedAt,
  rescanning,
  onRescan,
  boundSkillMap,
  selectedDirs,
  setSelectedDirs,
  slugOverrides,
  onSlugOverrideChange,
  allowLarge,
  setAllowLarge,
  importing,
  importResult,
  onImport,
  syncAction,
  syncStatus,
}: SourceInventoryProps) {
  const { t } = useLocale()
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)

  const trimmedQuery = query.trim()
  const filtered = useMemo(() => {
    if (trimmedQuery === '') return inventory
    const needle = trimmedQuery.toLowerCase()
    return inventory.filter((e) => e.name.toLowerCase().includes(needle))
  }, [inventory, trimmedQuery])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const currentPage = Math.min(page, totalPages)
  const visible = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)

  const summary = importResult?.summary
  const alertVariant = summary?.failed ? 'error' : summary?.skipped_conflict ? 'warning' : 'success'

  const handleQueryChange = (event: ChangeEvent<HTMLInputElement>) => {
    setQuery(event.target.value)
    setPage(1)
  }

  const handleClearQuery = () => {
    setQuery('')
    setPage(1)
  }

  const goToPage = (target: number) => (event: MouseEvent) => {
    event.preventDefault()
    setPage(Math.min(Math.max(target, 1), totalPages))
  }

  const pageItems = paginationItems(currentPage, totalPages)

  return (
    <div className="space-y-4">
      {/* One toolbar for everything you can do to this Source: refresh what
          it offers, pull its changes into the Store, or import from it. What
          each button does, and how old its reading is, lives on hover so the
          row stays a row. */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-lg font-semibold">{t('inventoryHeading', { count: inventory.length })}</h2>

            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={onRescan}
                    disabled={rescanning}
                    focusableWhenDisabled
                  >
                    {rescanning ? (
                      <Spinner data-icon="start" currentColor className="text-[1.2em]" />
                    ) : (
                      <Refresh data-icon="start" />
                    )}
                    {rescanning ? t('btnScanning') : t('btnRescan')}
                  </Button>
                }
              />
              <TooltipContent className="max-w-xs">
                {t('inventoryScopeNote')}
                <div className="mt-1 opacity-80">
                  {t('colLastScanned')}: <RelativeTime value={lastScannedAt} />
                </div>
              </TooltipContent>
            </Tooltip>

            {syncAction}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-xs select-none cursor-pointer">
              <Checkbox
                checked={allowLarge}
                onCheckedChange={(checked) => setAllowLarge(!!checked)}
                disabled={importing}
              />
              <span className="text-foreground-muted">{t('labelAllowLarge')}</span>
            </label>

            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={importing || selectedDirs.length === 0 || !available}
                    onClick={() => onImport({ all: false })}
                    focusableWhenDisabled
                  >
                    {importing && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
                    {t('btnImportSelected', { count: selectedDirs.length })}
                  </Button>
                }
              />
              <TooltipContent className="max-w-xs">{t('inventoryNote')}</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={importing || inventory.length === 0 || !available}
                    onClick={() => onImport({ all: true })}
                    focusableWhenDisabled
                  >
                    {importing && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
                    {t('btnImportAll')}
                  </Button>
                }
              />
              <TooltipContent className="max-w-xs">{t('inventoryNote')}</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {syncStatus}
      </div>

      {summary && (
        <Alert variant={alertVariant}>
          <AlertTitle>{t('alertImportCompleted')}</AlertTitle>
          <AlertDescription className="text-foreground-strong">
            {t('summaryText', {
              total: summary.total ?? 0,
              imported: summary.imported ?? 0,
              replaced: summary.replaced ?? 0,
              already_imported: summary.already_imported ?? 0,
              skipped_conflict: summary.skipped_conflict ?? 0,
              failed: summary.failed ?? 0,
            })}
          </AlertDescription>
        </Alert>
      )}

      {inventory.length === 0 ? (
        <p className="text-foreground-muted">{t('emptyInventory')}</p>
      ) : (
        <div className="space-y-4">
          <Input
            className="w-full max-w-xs"
            value={query}
            onChange={handleQueryChange}
            onClear={handleClearQuery}
            clearable
            startSlot={<Search />}
            placeholder={t('phSearchInventory')}
            aria-label={t('ariaSearchInventory')}
          />
          {filtered.length === 0 ? (
            <div role="status" className="rounded-xl border border-border bg-background p-8 text-center">
              <p className="text-foreground-muted">{t('emptyInventorySearch')}</p>
            </div>
          ) : (
            <>
              <SourceInventoryTable
                entries={visible}
                boundSkillMap={boundSkillMap}
                importResult={importResult}
                selectedDirs={selectedDirs}
                setSelectedDirs={setSelectedDirs}
                slugOverrides={slugOverrides}
                onSlugOverrideChange={onSlugOverrideChange}
                available={available}
                importing={importing}
              />
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
                            <PaginationLink active tabIndex={-1}>{item}</PaginationLink>
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
        </div>
      )}
    </div>
  )
}
