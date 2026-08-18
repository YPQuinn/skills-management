import { useCallback, useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchGroups, type GroupSummary } from './group-api'
import { CreateGroupForm } from './create-group-form'
import { useLocale } from './locale-context'

export function GroupsIndex() {
  const { t, formatTime, getErrorMessage } = useLocale()
  const [groups, setGroups] = useState<GroupSummary[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)

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
      </div>

      <CreateGroupForm onCreated={load} />

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadGroups')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadGroups')}</AlertDescription>
        </Alert>
      )}

      {loading && groups === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingGroups')} />
        </div>
      ) : groups === null ? null : groups.length === 0 ? (
        <p className="text-foreground-muted text-center py-8">{t('emptyGroupsIndex')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[600px]">
            <Table aria-label={t('captionGroupsIndex')}>
              <TableCaption className="sr-only">{t('captionGroupsIndex')}</TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead className="text-end">{t('colMemberCount')}</TableHead>
                  <TableHead>{t('labelCreated')}</TableHead>
                  <TableHead>{t('labelUpdated')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {groups.map((g) => (
                  <TableRow key={g.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link
                        to={`/groups/${encodeURIComponent(g.name)}`}
                        className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                      >
                        {g.name}
                      </Link>
                    </TableCell>
                    <TableCell className="text-end tabular-nums">{g.member_count}</TableCell>
                    <TableCell className="text-foreground-muted">{formatTime(g.created_at)}</TableCell>
                    <TableCell className="text-foreground-muted">{formatTime(g.updated_at)}</TableCell>
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
