import { Link } from 'react-router-dom'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import type { DesiredSkill } from './target-api'
import { SkillIdentity } from './skill-identity'
import { TermTooltip } from './term-tooltip'
import { useLocale } from './locale-context'

interface TargetDesiredSetSectionProps {
  desiredSkills: DesiredSkill[]
}

export function TargetDesiredSetSection({ desiredSkills }: TargetDesiredSetSectionProps) {
  const { t } = useLocale()

  return (
    <div className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div>
        <h2 className="text-lg font-semibold">
          <TermTooltip term={t('desiredSetHeading', { count: desiredSkills.length })} tip={t('tipDesiredSkillSet')} />
        </h2>
        <p className="text-sm text-foreground-muted">{t('desiredSetSubtitle')}</p>
      </div>

      {desiredSkills.length === 0 ? (
        <p className="text-foreground-muted text-sm py-4">{t('emptyDesiredSet')}</p>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[520px]">
            <Table aria-label={t('captionDesiredSetTable')}>
              <TableCaption className="sr-only">{t('captionDesiredSetTable')}</TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('colName')}</TableHead>
                  <TableHead>{t('colReasons')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {desiredSkills.map((ds) => (
                  <TableRow key={ds.id}>
                    <TableCell>
                      <SkillIdentity
                        name={ds.name}
                        slug={ds.slug}
                        nameNode={
                          <Link
                            to={`/skills/${encodeURIComponent(ds.slug)}`}
                            className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                          >
                            {ds.name}
                          </Link>
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <div className="space-y-1">
                        {ds.reasons.map((r, idx) => (
                          <div key={idx} className="text-xs flex items-center gap-1 text-foreground-muted">
                            <Badge variant="outline" className="text-[10px] px-1 py-0 font-normal">
                              {r.kind}
                            </Badge>
                            <span>
                              {r.kind === 'skill'
                                ? t('reasonDirect', { id: r.assignment_id })
                                : t('reasonGroup', {
                                    group: r.group_name || String(r.group_id),
                                    group_id: r.group_id || 0,
                                    id: r.assignment_id,
                                  })}
                            </span>
                          </div>
                        ))}
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
