import { Link } from 'react-router-dom'
import { Badge } from '@appica/ui-react/badge'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Checkbox } from '@appica/ui-react/checkbox'
import { CheckboxGroup } from '@appica/ui-react/checkbox-group'
import { SlugOverrideDialog } from './slug-override-dialog'
import type { SourceEntry } from './source-api'
import type { Skill, ImportResponse, ImportItemResult } from './skill-api'
import { useLocale } from './locale-context'

function truncate(value: string, max = 80): string {
  return value.length > max ? value.slice(0, max - 1) + '…' : value
}

interface SourceInventoryTableProps {
  entries: SourceEntry[]
  boundSkillMap: Map<string, Skill>
  importResult: ImportResponse | null
  selectedDirs: string[]
  setSelectedDirs: (dirs: string[]) => void
  slugOverrides: Record<string, string>
  onSlugOverrideChange: (dir: string, value: string) => void
  available: boolean
  importing: boolean
}

export function SourceInventoryTable({
  entries,
  boundSkillMap,
  importResult,
  selectedDirs,
  setSelectedDirs,
  slugOverrides,
  onSlugOverrideChange,
  available,
  importing,
}: SourceInventoryTableProps) {
  const { t } = useLocale()

  const resultItemMap = new Map<string, ImportItemResult>()
  if (importResult) {
    for (const item of importResult.items) {
      resultItemMap.set(item.relative_dir, item)
    }
  }

  const pageDirs = entries.map((e) => e.relative_dir)
  const selectedSet = new Set(selectedDirs)
  const selectedOnPage = pageDirs.filter((dir) => selectedSet.has(dir)).length
  const allSelectedOnPage = pageDirs.length > 0 && selectedOnPage === pageDirs.length

  const toggleSelectPage = (checked: boolean) => {
    setSelectedDirs(
      checked
        ? Array.from(new Set([...selectedDirs, ...pageDirs]))
        : selectedDirs.filter((dir) => !pageDirs.includes(dir)),
    )
  }

  return (
    <CheckboxGroup aria-label={t('ariaInventorySelection')} value={selectedDirs} onValueChange={setSelectedDirs}>
      <ScrollArea className="w-full" orientation="horizontal">
        <div className="min-w-[800px]">
          <Table aria-label={t('captionSourceInventory')}>
            <TableCaption className="sr-only">{t('captionSourceInventory')}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead className="sticky left-0 z-20 w-10 border-e border-border bg-background-muted">
                  <Checkbox
                    checked={allSelectedOnPage}
                    indeterminate={!allSelectedOnPage && selectedOnPage > 0}
                    onCheckedChange={(checked) => toggleSelectPage(!!checked)}
                    aria-label={t('ariaSelectPageInventory', { count: entries.length })}
                    disabled={importing || !available}
                  />
                </TableHead>
                <TableHead>{t('navSkills')}</TableHead>
                <TableHead>{t('colSlugOverride')}</TableHead>
                <TableHead>{t('colStatus')}</TableHead>
                <TableHead>{t('colDescription')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((e) => {
                const boundSkill = boundSkillMap.get(e.relative_dir)
                const itemResult = resultItemMap.get(e.relative_dir)
                return (
                  <TableRow key={e.relative_dir}>
                    <TableCell className="sticky left-0 z-10 border-e border-border bg-background">
                      <Checkbox name={e.relative_dir} aria-label={t('ariaSelectItem', { name: e.name })} disabled={importing || !available} />
                    </TableCell>
                    <TableCell className="font-medium text-foreground-strong">
                      {boundSkill ? (
                        <Link
                          to={`/skills/${encodeURIComponent(boundSkill.slug)}`}
                          className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                        >
                          {e.name}
                        </Link>
                      ) : (
                        e.name
                      )}
                      <div className="mt-0.5 font-mono text-xs font-normal text-foreground-muted">{e.relative_dir}</div>
                    </TableCell>
                    <TableCell>
                      <SlugOverrideDialog
                        name={e.name}
                        relativeDir={e.relative_dir}
                        value={slugOverrides[e.relative_dir] || ''}
                        disabled={importing || !available}
                        onSave={(value) => onSlugOverrideChange(e.relative_dir, value)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1 items-start">
                        {boundSkill && !itemResult && (
                          <Badge variant="success" className="text-xs">
                            {t('statusImported')}
                          </Badge>
                        )}
                        {!boundSkill && !itemResult && (
                          <Badge variant="soft" className="text-xs">
                            {t('statusAvailable')}
                          </Badge>
                        )}
                        {itemResult && (
                          <div className="space-y-1">
                            {itemResult.status === 'imported' && (
                              <Badge variant="success" className="text-xs">
                                {t('statusImported')}
                              </Badge>
                            )}
                            {itemResult.status === 'already_imported' && (
                              <Badge variant="soft" className="text-xs">
                                {t('statusAlreadyImported')}
                              </Badge>
                            )}
                            {itemResult.status === 'replaced' && (
                              <Badge variant="success" className="text-xs">
                                {t('statusReplaced')}
                              </Badge>
                            )}
                            {itemResult.status === 'skipped_conflict' && (
                              <Badge variant="warning" className="text-xs">
                                {t('statusConflict')}
                              </Badge>
                            )}
                            {itemResult.status === 'failed' && (
                              <Badge variant="error" className="text-xs">
                                {t('statusFailed')}
                              </Badge>
                            )}
                            {itemResult.slug && (
                              <Link
                                to={`/skills/${encodeURIComponent(itemResult.slug)}`}
                                className="block text-xs underline text-foreground-muted hover:text-foreground"
                              >
                                {t('linkViewSlug', { slug: itemResult.slug })}
                              </Link>
                            )}
                            {itemResult.message && (
                              <span className="block text-xs text-foreground-muted">{itemResult.message}</span>
                            )}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-foreground-muted">{truncate(e.description)}</TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </ScrollArea>
    </CheckboxGroup>
  )
}
