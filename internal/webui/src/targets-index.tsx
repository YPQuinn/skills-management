import { useCallback, useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { ListPageSkeleton } from './list-page-skeleton'
import {
  fetchTargetAdapters,
  fetchTargets,
  type TargetAdapter,
  type TargetSummary,
} from './target-api'
import { RegisterTargetForm } from './register-target-form'
import { ResourceCreateDialog } from './resource-create-dialog'
import { outcomeBadgeVariant, outcomeKey } from './target-distribution-labels'
import { adapterLabel, scopeLabel } from './target-labels'
import { CopyablePath } from './copyable-path'
import { useLocale } from './locale-context'

export function TargetsIndex() {
  const { t, getErrorMessage } = useLocale()
  const [adapters, setAdapters] = useState<TargetAdapter[]>([])
  const [targets, setTargets] = useState<TargetSummary[] | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown | null>(null)

  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)

    Promise.all([
      fetchTargetAdapters(controller.signal).catch(() => []),
      fetchTargets(controller.signal),
    ])
      .then(([adList, tList]) => {
        if (inflight.current === controller) {
          setAdapters(adList)
          setTargets(tList)
        }
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
          <h1 className="text-2xl font-bold">{t('targetsTitle')}</h1>
          <p className="text-foreground-muted text-sm">{t('targetsSubtitle')}</p>
        </div>
        <ResourceCreateDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          title={t('registerTargetTitle')}
          description={t('registerTargetSubtitle')}
          triggerLabel={t('btnAddTarget')}
        >
          <RegisterTargetForm
            adapters={adapters}
            onRegistered={() => {
              setCreateOpen(false)
              load()
            }}
          />
        </ResourceCreateDialog>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadTargets')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadTargets')}</AlertDescription>
        </Alert>
      )}

      {/* Registered Targets List */}
      {loading && targets === null ? (
        <ListPageSkeleton label={t('ariaLoadingTargets')} />
      ) : targets === null ? null : targets.length === 0 ? (
        <p className="text-foreground-muted text-center py-8">{t('emptyTargetsIndex')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[700px]">
            <Table aria-label={t('captionTargetsIndex')}>
              <TableCaption className="sr-only">{t('captionTargetsIndex')}</TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead>{t('colAdapter')}</TableHead>
                  <TableHead>{t('colScope')}</TableHead>
                  <TableHead>{t('colPath')}</TableHead>
                  <TableHead>{t('colLastResult')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {targets.map((tgt) => (
                  <TableRow key={tgt.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link
                        to={`/targets/${encodeURIComponent(tgt.name)}`}
                        className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                      >
                        {tgt.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge variant="soft">{adapterLabel(tgt.adapter, adapters, t)}</Badge>
                    </TableCell>
                    <TableCell className="text-foreground-muted text-xs">
                      {scopeLabel(tgt.scope, t)}
                      {tgt.project_root ? <span className="font-mono"> ({tgt.project_root})</span> : null}
                    </TableCell>
                    <TableCell className="font-mono text-foreground-muted text-xs" title={tgt.path}>
                      <CopyablePath value={tgt.path} className="text-xs" />
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1">
                        {tgt.last_result ? (
                          <Badge variant={outcomeBadgeVariant(tgt.last_result)}>
                            {t(outcomeKey(tgt.last_result))}
                          </Badge>
                        ) : (
                          <span className="text-foreground-muted text-xs">{t('distributionNever')}</span>
                        )}
                        {tgt.stale && (
                          <Badge variant="warning" className="text-xs">
                            {t('distributionStale')}
                          </Badge>
                        )}
                      </div>
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
