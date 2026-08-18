import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Input } from '@appica/ui-react/input'
import { Pagination, PaginationList, PaginationItem, PaginationLink, PaginationEllipsis } from '@appica/ui-react/pagination'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ArrowRight, ChevronLeft, ChevronRight, Search } from '@appica/icons-react'
import { fetchSkills } from './skill-api'
import type { Skill } from './skill-api'
import { paginationItems } from './pagination-items'
import { useLocale } from './locale-context'

const PAGE_SIZE = 10

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillsIndex() {
  const { t, formatTime, getErrorMessage } = useLocale()
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)

  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)
    fetchSkills(controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSkills(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(err)
      })
      .finally(() => {
        if (inflight.current === controller) setLoading(false)
      })
  }, [])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  const trimmedQuery = query.trim()
  const filteredSkills = useMemo(() => {
    if (skills === null) return []
    if (trimmedQuery === '') return skills
    const needle = trimmedQuery.toLowerCase()
    return skills.filter((skill) => skill.name.toLowerCase().includes(needle))
  }, [skills, trimmedQuery])

  const totalPages = Math.max(1, Math.ceil(filteredSkills.length / PAGE_SIZE))
  const currentPage = Math.min(page, totalPages)
  const visibleSkills = filteredSkills.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)

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
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t('skillsTitle')}</h1>
          <p className="text-foreground-muted text-sm">{t('skillsSubtitle')}</p>
        </div>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadSkills')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadSkills')}</AlertDescription>
        </Alert>
      )}

      {loading && skills === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingSkills')} />
        </div>
      ) : skills === null ? null : skills.length === 0 ? (
        <div className="rounded-xl border border-border bg-background p-8 text-center space-y-3">
          <p className="text-foreground-muted">{t('emptySkillsIndexTitle')}</p>
          <p className="text-sm text-foreground-muted">{t('emptySkillsIndexSubtitle')}</p>
          <Link
            to="/sources"
            className="inline-flex items-center gap-1 text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {t('linkGoToSources')}
            <ArrowRight className="size-4" />
          </Link>
        </div>
      ) : (
        <div className="space-y-4">
          <Input
            className="w-full max-w-xs"
            value={query}
            onChange={handleQueryChange}
            onClear={handleClearQuery}
            clearable
            startSlot={<Search />}
            placeholder={t('phSearchSkills')}
            aria-label={t('ariaSearchSkills')}
          />
          {filteredSkills.length === 0 ? (
            <div role="status" className="rounded-xl border border-border bg-background p-8 text-center">
              <p className="text-foreground-muted">{t('emptySkillsSearch')}</p>
            </div>
          ) : (
            <>
              <ScrollArea className="w-full" orientation="horizontal">
                <div className="min-w-[700px]">
                  <Table>
                    <TableCaption className="sr-only">{t('captionSkillStore')}</TableCaption>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('colName')}</TableHead>
                        <TableHead>{t('colSlug')}</TableHead>
                        <TableHead>{t('colDescription')}</TableHead>
                        <TableHead>{t('colSourceBinding')}</TableHead>
                        <TableHead>{t('colUpdated')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {visibleSkills.map((s) => (
                        <TableRow key={s.id}>
                          <TableCell className="font-medium text-foreground-strong">
                            <Link
                              to={`/skills/${encodeURIComponent(s.slug)}`}
                              className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                            >
                              {s.name}
                            </Link>
                          </TableCell>
                          <TableCell className="font-mono text-foreground-muted">{s.slug}</TableCell>
                          <TableCell className="text-foreground-muted">{s.description}</TableCell>
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
                              <Badge variant="outline" className="text-xs">
                                {t('badgeUnbound')}
                              </Badge>
                            )}
                          </TableCell>
                          <TableCell className="text-foreground-muted text-xs">{formatTime(s.updated_at)}</TableCell>
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
