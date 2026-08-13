import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import type { SourceIssue } from './source-api'
import { useLocale } from './locale-context'

export function SourceIssuesList({ issues }: { issues: SourceIssue[] }) {
  const { t } = useLocale()
  if (issues.length === 0) return null

  return (
    <div>
      <Alert variant="warning">
        <AlertTitle>
          {t('alertSkippedEntriesTitle', {
            count: issues.length,
            entries: issues.length === 1 ? t('entrySingular') : t('entryPlural'),
          })}
        </AlertTitle>
        <AlertDescription>{t('descSkippedEntries')}</AlertDescription>
      </Alert>
      <ul className="list-disc pl-4 mt-2 space-y-1 text-sm">
        {issues.map((i) => (
          <li key={i.relative_dir}>
            <span className="font-mono">{i.relative_dir}</span>: {i.reason}
          </li>
        ))}
      </ul>
    </div>
  )
}
