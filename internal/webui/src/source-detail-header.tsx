import { Link } from 'react-router-dom'
import { ArrowLeft } from '@appica/icons-react'
import { SourceStatusBadge } from './source-status'
import type { SourceDetail } from './source-api'
import { SourceLocationIcon } from './source-location'
import { SourceDeleteAction } from './source-delete-action'
import { useLocale } from './locale-context'

export function SourceDetailHeader({
  source,
  sourceId,
}: {
  source: SourceDetail
  sourceId: number | null
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
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-bold">{source.name}</h1>
          <SourceStatusBadge available={source.available} stale={source.stale} size="md" />
        </div>
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
      {sourceId !== null && <SourceDeleteAction sourceId={sourceId} name={source.name} />}
    </div>
  )
}
