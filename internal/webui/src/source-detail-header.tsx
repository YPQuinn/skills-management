import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Spinner } from '@appica/ui-react/spinner'
import { ArrowLeft } from '@appica/icons-react'
import { SourceStatusBadge } from './source-status'
import type { SourceDetail } from './source-api'
import { SourceLocationIcon } from './source-location'
import { SourceDeleteAction } from './source-delete-action'
import { useLocale } from './locale-context'

export function SourceDetailHeader({
  source,
  sourceId,
  checking,
  onCheck,
}: {
  source: SourceDetail
  sourceId: number | null
  checking: boolean
  onCheck: () => void
}) {
  const { t } = useLocale()
  const isURL = /^https?:\/\//i.test(source.location)
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <Link
          to="/sources"
          className="inline-flex items-center gap-1 text-sm text-foreground-muted underline decoration-border underline-offset-2 hover:decoration-foreground"
        >
          <ArrowLeft className="size-4" />
          {t('linkAllSources')}
        </Link>
        <h1 className="text-2xl font-bold mt-1">{source.name}</h1>
        <p className="mt-0.5 flex items-start gap-1.5 text-sm text-foreground-muted break-all">
          <SourceLocationIcon source={source} />
          {isURL ? (
            <a
              href={source.location}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 underline decoration-border underline-offset-2 hover:decoration-foreground"
            >
              {source.location}
            </a>
          ) : (
            <span className="min-w-0">{source.location}</span>
          )}
        </p>
      </div>
      <div className="flex flex-col items-end gap-2">
        <SourceStatusBadge available={source.available} stale={source.stale} />
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={onCheck} disabled={checking} focusableWhenDisabled>
            {checking && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
            {checking ? t('btnChecking') : t('btnCheckAgain')}
          </Button>
          {sourceId !== null && <SourceDeleteAction sourceId={sourceId} name={source.name} />}
        </div>
      </div>
    </div>
  )
}
