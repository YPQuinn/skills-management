import type { MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { Pagination, PaginationList, PaginationItem, PaginationLink, PaginationEllipsis } from '@appica/ui-react/pagination'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ChevronLeft, ChevronRight, Clock, Unlink } from '@appica/icons-react'
import type { Skill } from './skill-api'
import { StatusLabel } from './status-label'
import { SyncStatusBadge } from './sync-status'
import { useLocale } from './locale-context'
import { SkillIdentity } from './skill-identity'
import { RelativeTime } from './relative-time'

export function SkillsIndexTable({
  skills,
  totalPages,
  currentPage,
  pageItems,
  onPage,
}: {
  skills: Skill[]
  totalPages: number
  currentPage: number
  pageItems: (number | 'gap')[]
  onPage: (target: number) => (event: MouseEvent) => void
}) {
  const { t } = useLocale()
  return (
    <>
      <ScrollArea className="w-full" orientation="horizontal">
        <div className="w-full min-w-[720px]">
          <Table>
            <TableCaption className="sr-only">{t('captionSkillStore')}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead className="w-px whitespace-nowrap">{t('colName')}</TableHead>
                <TableHead>{t('colDescription')}</TableHead>
                <TableHead className="w-px whitespace-nowrap">{t('colSyncStatus')}</TableHead>
                <TableHead className="w-px whitespace-nowrap">{t('colSourceBinding')}</TableHead>
                <TableHead className="w-px whitespace-nowrap">{t('colUpdated')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills.map((s) => (
                <TableRow key={s.id}>
                  <TableCell className="w-px whitespace-nowrap">
                    <SkillIdentity
                      name={s.name}
                      slug={s.slug}
                      nameNode={
                        <Link
                          to={`/skills/${encodeURIComponent(s.slug)}`}
                          className="whitespace-nowrap underline decoration-border underline-offset-2 hover:decoration-foreground"
                        >
                          {s.name}
                        </Link>
                      }
                    />
                  </TableCell>
                  <TableCell className="max-w-0 text-foreground-muted">
                    <span className="line-clamp-2">{s.description}</span>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1">
                      <SyncStatusBadge status={s.sync_status} />
                      {s.sync_stale && (
                        <StatusLabel icon={Clock} tone="warning">
                          {t('statusSyncStale')}
                        </StatusLabel>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {s.binding ? (
                      <div className="flex flex-col text-xs">
                        <Link
                          to={`/sources/${encodeURIComponent(s.binding.source_name)}`}
                          className="font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
                        >
                          {s.binding.source_name}
                        </Link>
                        <span className="font-mono text-foreground-muted">{s.binding.relative_dir}</span>
                      </div>
                    ) : (
                      <StatusLabel icon={Unlink} tone="muted">
                        {t('badgeUnbound')}
                      </StatusLabel>
                    )}
                  </TableCell>
                  <TableCell className="text-foreground-muted text-xs">
                    <RelativeTime value={s.updated_at} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </ScrollArea>
      {totalPages > 1 && (
        <div className="max-w-full overflow-x-auto">
          <Pagination aria-label={t('ariaPagination')}>
            <PaginationList>
              <PaginationItem>
                <PaginationLink href="#!" aria-label={t('ariaPreviousPage')} className="px-0" disabled={currentPage === 1} onClick={onPage(currentPage - 1)}>
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
                    <PaginationLink href="#!" aria-label={t('ariaGoToPage', { page: item })} onClick={onPage(item)}>
                      {item}
                    </PaginationLink>
                  </PaginationItem>
                ),
              )}
              <PaginationItem>
                <PaginationLink href="#!" aria-label={t('ariaNextPage')} className="px-0" disabled={currentPage === totalPages} onClick={onPage(currentPage + 1)}>
                  <ChevronRight />
                </PaginationLink>
              </PaginationItem>
            </PaginationList>
          </Pagination>
        </div>
      )}
    </>
  )
}
