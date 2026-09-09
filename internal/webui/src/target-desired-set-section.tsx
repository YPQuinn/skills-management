import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@appica/ui-react/dialog'
import { Eye } from '@appica/icons-react'
import type { DesireReason, DesiredSkill } from './target-api'
import type { DictionaryKey } from './locale-dictionary'
import { SkillIdentity } from './skill-identity'
import { useLocale } from './locale-context'

interface TargetDesiredSetPreviewProps {
  desiredSkills: DesiredSkill[]
}

function skillSourceLine(
  reasons: DesireReason[],
  t: (key: DictionaryKey, params?: Record<string, string | number>) => string,
): string {
  return reasons
    .map((r) =>
      r.kind === 'skill'
        ? t('reasonDirect', { id: r.assignment_id })
        : t('reasonGroup', {
            group: r.group_name || String(r.group_id),
            group_id: r.group_id || 0,
            id: r.assignment_id,
          }),
    )
    .join(' · ')
}

export function TargetDesiredSetPreview({ desiredSkills }: TargetDesiredSetPreviewProps) {
  const { t } = useLocale()
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button variant="outline" size="sm" aria-label={t('ariaPreviewDesiredSet')} onClick={() => setOpen(true)}>
        <Eye data-icon="start" />
        {t('btnPreview')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl" closeLabel={t('btnClose')}>
          <DialogHeader>
            <DialogTitle>{t('desiredSetHeading')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="max-h-[min(70vh,36rem)] overflow-y-auto">
            {desiredSkills.length === 0 ? (
              <p className="text-foreground-muted text-sm py-2">{t('emptyDesiredSet')}</p>
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
                          <TableCell className="text-sm text-foreground-muted whitespace-nowrap">
                            {skillSourceLine(ds.reasons, t)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </ScrollArea>
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  )
}
