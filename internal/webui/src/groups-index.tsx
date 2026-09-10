import { useCallback, useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Thumbnail } from '@appica/ui-react/thumbnail'
import { ChevronRight, Clock, Folder } from '@appica/icons-react'
import { ListPageSkeleton } from './list-page-skeleton'
import { fetchGroups, type GroupSummary } from './group-api'
import { CreateGroupForm } from './create-group-form'
import { ResourceCreateDialog } from './resource-create-dialog'
import { useLocale } from './locale-context'
import { RelativeTime } from './relative-time'

export function GroupsIndex() {
  const { t, getErrorMessage } = useLocale()
  const [groups, setGroups] = useState<GroupSummary[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)
    fetchGroups(controller.signal)
      .then((data) => {
        if (inflight.current === controller) setGroups(data)
      })
      .catch((err: unknown) => {
        if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t('groupsTitle')}</h1>
          <p className="text-foreground-muted text-sm">{t('groupsSubtitle')}</p>
        </div>
        <ResourceCreateDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          title={t('createGroupTitle')}
          description={t('createGroupSubtitle')}
          triggerLabel={t('btnCreateGroup')}
        >
          <CreateGroupForm onCreated={() => { setCreateOpen(false); load() }} />
        </ResourceCreateDialog>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadGroups')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadGroups')}</AlertDescription>
        </Alert>
      )}

      {loading && groups === null ? (
        <ListPageSkeleton label={t('ariaLoadingGroups')} />
      ) : groups === null ? null : groups.length === 0 ? (
        <p className="text-foreground-muted text-center py-8">{t('emptyGroupsIndex')}</p>
      ) : (
        <ul aria-label={t('captionGroupsIndex')} className="space-y-3">
          {groups.map((group) => (
            <li key={group.id}>
              <Link
                to={`/groups/${encodeURIComponent(group.name)}`}
                aria-label={group.name}
                className="flex items-center gap-4 rounded-xl border border-border bg-background px-5 py-4 shadow-sm transition hover:bg-background-muted hover:shadow-md"
              >
                <Thumbnail variant="icon-primary" size="lg" shape="rounded" aria-hidden>
                  <Folder />
                </Thumbnail>
                <div className="min-w-0 flex-1">
                  <div className="truncate font-semibold text-foreground-strong">{group.name}</div>
                  <div className="mt-1 flex items-center gap-1 text-sm text-foreground-muted">
                    <Clock className="size-3.5 shrink-0" />
                    <RelativeTime value={group.updated_at} />
                  </div>
                </div>
                <div className="text-end">
                  <div className="text-2xl font-semibold tabular-nums text-foreground-strong">{group.member_count}</div>
                  <div className="text-xs text-foreground-muted">{t('colSkillCount')}</div>
                </div>
                <ChevronRight className="size-5 shrink-0 text-foreground-muted" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
