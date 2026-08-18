import { useCallback, useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Button } from '@appica/ui-react/button'
import { Check, X, InfoCircle, Plus } from '@appica/icons-react'
import {
  fetchTargetAdapters,
  fetchTargets,
  type TargetAdapter,
  type TargetSummary,
} from './target-api'
import { RegisterTargetForm } from './register-target-form'
import { useLocale } from './locale-context'

export function TargetsIndex() {
  const { t, formatTime, getErrorMessage } = useLocale()
  const [adapters, setAdapters] = useState<TargetAdapter[]>([])
  const [targets, setTargets] = useState<TargetSummary[] | null>(null)
  const [selectedAdapterKey, setSelectedAdapterKey] = useState<string | undefined>(undefined)
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
      </div>

      {/* Built-in Adapters & Detection */}
      <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
        <div>
          <h2 className="text-lg font-semibold">{t('adaptersTitle')}</h2>
          <p className="text-sm text-foreground-muted">{t('adaptersSubtitle')}</p>
        </div>

        {adapters.length > 0 && (
          <ScrollArea className="w-full" orientation="horizontal">
            <div className="min-w-[650px]">
              <Table aria-label={t('captionAdaptersTable')}>
                <TableCaption className="sr-only">{t('captionAdaptersTable')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('colName')}</TableHead>
                    <TableHead>{t('colAdapter')}</TableHead>
                    <TableHead>{t('colDetectionStatus')}</TableHead>
                    <TableHead>{t('colEvidence')}</TableHead>
                    <TableHead className="text-end">{t('colActions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {adapters.map((ad) => {
                    const status = ad.detection.status
                    const isDetected = status === 'detected'
                    const isNotDetected = status === 'not_detected' || status === 'not_found'
                    const isNotApplicable = status === 'not_applicable'

                    let statusLabel = t('statusUnknown')
                    if (isDetected) statusLabel = t('statusDetected')
                    else if (isNotDetected) statusLabel = t('statusNotDetected')
                    else if (isNotApplicable) statusLabel = t('statusNotApplicable')
                    else if (status === 'unknown') statusLabel = t('statusUnknown')

                    return (
                      <TableRow key={ad.key}>
                        <TableCell className="font-medium text-foreground-strong">{ad.name}</TableCell>
                        <TableCell className="font-mono text-foreground-muted">{ad.key}</TableCell>
                        <TableCell>
                          <Badge variant={isDetected ? 'soft' : 'outline'} className="gap-1">
                            {isDetected ? (
                              <Check className="size-3 text-success inline" />
                            ) : isNotDetected ? (
                              <X className="size-3 text-foreground-muted inline" />
                            ) : (
                              <InfoCircle className="size-3 inline" />
                            )}
                            {statusLabel}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-foreground-muted text-xs">
                          {ad.detection.evidence && ad.detection.evidence.length > 0
                            ? ad.detection.evidence.join('; ')
                            : '—'}
                        </TableCell>
                        <TableCell className="text-end">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedAdapterKey(ad.key)}
                          >
                            <Plus className="size-4" />
                            {t('btnRegisterAdapter')}
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

      {/* Registration Form */}
      <RegisterTargetForm
        adapters={adapters}
        initialAdapter={selectedAdapterKey}
        onRegistered={load}
      />

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadTargets')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'alertCouldNotLoadTargets')}</AlertDescription>
        </Alert>
      )}

      {/* Registered Targets List */}
      {loading && targets === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingTargets')} />
        </div>
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
                  <TableHead>{t('labelCreated')}</TableHead>
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
                      <Badge variant="soft" className="uppercase font-mono">
                        {tgt.adapter}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-foreground-muted text-xs">
                      {tgt.scope}
                      {tgt.project_root ? ` (${tgt.project_root})` : ''}
                    </TableCell>
                    <TableCell className="font-mono text-foreground-muted text-xs" title={tgt.path}>
                      {tgt.path}
                    </TableCell>
                    <TableCell className="text-foreground-muted text-xs">
                      {formatTime(tgt.created_at)}
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
