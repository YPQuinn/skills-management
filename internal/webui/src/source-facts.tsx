import type { ReactNode } from 'react'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@appica/ui-react/collapsible'
import { ChevronRight } from '@appica/icons-react'
import type { SourceDetail } from './source-api'
import { useLocale } from './locale-context'

function FactLine({ label, value }: { label: string; value: ReactNode }) {
  return (
    <p>
      <span className="text-foreground-muted">{label}: </span>
      <span className="break-all">{value}</span>
    </p>
  )
}

export function SourceFacts({ source }: { source: SourceDetail }) {
  const { t, formatTime } = useLocale()

  return (
    <div className="space-y-2 text-xs">
      <p className="text-foreground-muted">
        {t('colKind')}: {source.kind}
        <span aria-hidden="true"> · </span>
        {t('colLastChecked')}: {formatTime(source.last_checked_at)}
      </p>

      <Collapsible>
        <CollapsibleTrigger className="group inline-flex items-center gap-1 text-foreground-muted underline decoration-border underline-offset-2 transition-colors hover:text-foreground hover:decoration-foreground">
          <ChevronRight className="size-3.5 shrink-0 stroke-2 transition-transform duration-200 group-data-panel-open:rotate-90 motion-reduce:transition-none" />
          {t('showSourceDetails')}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="space-y-1 pt-2">
            <FactLine label={t('colKind')} value={source.kind} />
            <FactLine label={t('labelRef')} value={source.ref || t('factRefDefault')} />
            <FactLine label={t('labelSubpath')} value={source.subpath || t('factSubpathDefault')} />
            {source.kind === 'git' && (
              <FactLine
                label={t('labelResolvedCommit')}
                value={<span className="font-mono">{source.last_commit || '—'}</span>}
              />
            )}
            <FactLine label={t('colLastChecked')} value={formatTime(source.last_checked_at)} />
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
