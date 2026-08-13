import type { SourceDetail } from './source-api'
import { useLocale } from './locale-context'

export function SourceFactsGrid({ source }: { source: SourceDetail }) {
  const { t, formatTime } = useLocale()

  return (
    <div className="grid gap-4 sm:grid-cols-3 text-sm">
      <div className="border border-border rounded-xl p-4 bg-background">
        <div className="text-foreground-subtle">{t('colKind')}</div>
        <div className="font-medium mt-0.5">{source.kind}</div>
      </div>
      <div className="border border-border rounded-xl p-4 bg-background">
        <div className="text-foreground-subtle">{t('labelRef')}</div>
        <div className="font-medium mt-0.5">{source.ref || t('factRefDefault')}</div>
      </div>
      <div className="border border-border rounded-xl p-4 bg-background">
        <div className="text-foreground-subtle">{t('labelSubpath')}</div>
        <div className="font-medium mt-0.5">{source.subpath || t('factSubpathDefault')}</div>
      </div>
      {source.kind === 'git' && (
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelResolvedCommit')}</div>
          <div className="font-medium mt-0.5 font-mono break-all">{source.last_commit || '—'}</div>
        </div>
      )}
      <div className="border border-border rounded-xl p-4 bg-background">
        <div className="text-foreground-subtle">{t('colLastChecked')}</div>
        <div className="font-medium mt-0.5">{formatTime(source.last_checked_at)}</div>
      </div>
    </div>
  )
}
