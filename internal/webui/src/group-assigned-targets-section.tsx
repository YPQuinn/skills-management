import { Link } from 'react-router-dom'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { ArrowRight, Clock, Target } from '@appica/icons-react'
import type { GroupTarget } from './group-api'
import type { TargetAdapter } from './target-api'
import { adapterLabel, scopeLabel } from './target-labels'
import { outcomeKey, outcomeMark } from './target-distribution-labels'
import { CopyablePath } from './copyable-path'
import { StatusLabel } from './status-label'
import { useLocale } from './locale-context'

export function GroupAssignedTargetsSection({
  targets,
  adapters,
}: {
  targets: GroupTarget[]
  adapters: TargetAdapter[]
}) {
  const { t } = useLocale()

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Target className="size-5 text-foreground-muted" />
          {t('groupAssignedTargetsHeading', { count: targets.length })}
        </h2>
        {targets.length > 0 && (
          <p className="text-foreground-muted text-sm mt-1">{t('groupAssignedTargetsSubtitle')}</p>
        )}
      </div>
      {targets.length === 0 ? (
        <div className="space-y-3 py-2">
          <p className="text-foreground-muted text-sm">{t('emptyGroupAssignedTargets')}</p>
          <Link
            to="/targets"
            className="inline-flex items-center gap-1 text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {t('linkGoToTargets')}
            <ArrowRight className="size-4" />
          </Link>
        </div>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[700px]">
            <Table aria-label={t('groupAssignedTargetsHeading', { count: targets.length })}>
              <TableCaption className="sr-only">
                {t('groupAssignedTargetsHeading', { count: targets.length })}
              </TableCaption>
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
                {targets.map((target) => (
                  <TableRow key={target.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link
                        to={`/targets/${encodeURIComponent(target.name)}`}
                        className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                      >
                        {target.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge variant="soft">{adapterLabel(target.adapter, adapters, t)}</Badge>
                    </TableCell>
                    <TableCell className="text-foreground-muted text-xs">
                      {scopeLabel(target.scope, t)}
                      {target.project_root ? <span className="font-mono"> ({target.project_root})</span> : null}
                    </TableCell>
                    <TableCell className="font-mono text-foreground-muted text-xs" title={target.path}>
                      <CopyablePath value={target.path} className="text-xs" />
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1">
                        {target.last_result ? (
                          <StatusLabel {...outcomeMark(target.last_result)}>
                            {t(outcomeKey(target.last_result))}
                          </StatusLabel>
                        ) : (
                          <span className="text-foreground-muted text-xs">{t('distributionNever')}</span>
                        )}
                        {target.stale && (
                          <StatusLabel icon={Clock} tone="warning">
                            {t('distributionStale')}
                          </StatusLabel>
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
