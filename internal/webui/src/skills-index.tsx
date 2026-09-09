import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { ListPageSkeleton } from './list-page-skeleton'
import { ArrowRight, Search } from '@appica/icons-react'
import { fetchSkills, type Skill } from './skill-api'
import { paginationItems } from './pagination-items'
import { SyncFilterChips } from './sync-filter-chips'
import { skillMatchesSyncFilter, type SyncFilter } from './sync-filter'
import { useLocale } from './locale-context'
import { SkillsIndexTable } from './skills-index-table'

const PAGE_SIZE = 10

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillsIndex() {
  const { t, getErrorMessage } = useLocale()
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [syncFilter, setSyncFilter] = useState<SyncFilter>('all')
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
    const needle = trimmedQuery.toLowerCase()
    return skills.filter((skill) => {
      if (!skillMatchesSyncFilter(skill, syncFilter)) return false
      if (needle === '') return true
      return skill.name.toLowerCase().includes(needle)
    })
  }, [skills, trimmedQuery, syncFilter])

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

  const handleSyncFilterChange = (next: SyncFilter) => {
    setSyncFilter(next)
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
        <ListPageSkeleton label={t('ariaLoadingSkills')} />
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
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
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
            <SyncFilterChips skills={skills} value={syncFilter} onChange={handleSyncFilterChange} />
          </div>
          {filteredSkills.length === 0 ? (
            <div role="status" className="rounded-xl border border-border bg-background p-8 text-center">
              <p className="text-foreground-muted">{t('emptySkillsSearch')}</p>
            </div>
          ) : (
            <SkillsIndexTable
              skills={visibleSkills}
              totalPages={totalPages}
              currentPage={currentPage}
              pageItems={pageItems}
              onPage={goToPage}
            />
          )}
        </div>
      )}
    </div>
  )
}
