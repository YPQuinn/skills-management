import { Link } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Badge } from '@appica/ui-react/badge'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Table, TableCaption, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { Spinner } from '@appica/ui-react/spinner'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Checkbox } from '@appica/ui-react/checkbox'
import { CheckboxGroup } from '@appica/ui-react/checkbox-group'
import { Input } from '@appica/ui-react/input'
import type { SourceEntry } from './source-api'
import type { Skill, ImportResponse, ImportItemResult } from './skill-api'
import { useLocale } from './locale-context'

function truncate(value: string, max = 80): string {
  return value.length > max ? value.slice(0, max - 1) + '…' : value
}

interface SourceInventoryProps {
  inventory: SourceEntry[]
  available: boolean
  boundSkillMap: Map<string, Skill>
  selectedDirs: string[]
  setSelectedDirs: (dirs: string[]) => void
  slugOverrides: Record<string, string>
  onSlugOverrideChange: (dir: string, value: string) => void
  allowLarge: boolean
  setAllowLarge: (val: boolean) => void
  importing: boolean
  importResult: ImportResponse | null
  onImport: (options: { all?: boolean }) => void
}

export function SourceInventory({
  inventory,
  available,
  boundSkillMap,
  selectedDirs,
  setSelectedDirs,
  slugOverrides,
  onSlugOverrideChange,
  allowLarge,
  setAllowLarge,
  importing,
  importResult,
  onImport,
}: SourceInventoryProps) {
  const { t } = useLocale()
  const allRelativeDirs = inventory.map((e) => e.relative_dir)

  const resultItemMap = new Map<string, ImportItemResult>()
  if (importResult) {
    for (const item of importResult.items) {
      resultItemMap.set(item.relative_dir, item)
    }
  }

  const summary = importResult?.summary
  const alertVariant = summary?.failed ? 'error' : summary?.skipped_conflict ? 'warning' : 'success'

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-lg font-semibold">{t('inventoryHeading', { count: inventory.length })}</h2>

          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-sm select-none cursor-pointer">
              <Checkbox
                checked={allowLarge}
                onCheckedChange={(checked) => setAllowLarge(!!checked)}
                disabled={importing}
              />
              <span className="text-foreground-subtle">{t('labelAllowLarge')}</span>
            </label>

            <Button
              variant="outline"
              size="sm"
              disabled={importing || selectedDirs.length === 0 || !available}
              onClick={() => onImport({ all: false })}
              focusableWhenDisabled
            >
              {importing && <Spinner data-icon="start" currentColor />}
              {t('btnImportSelected', { count: selectedDirs.length })}
            </Button>

            <Button
              variant="primary"
              size="sm"
              disabled={importing || inventory.length === 0 || !available}
              onClick={() => onImport({ all: true })}
              focusableWhenDisabled
            >
              {importing && <Spinner data-icon="start" currentColor />}
              {t('btnImportAll')}
            </Button>
          </div>
        </div>

        <p className="text-xs text-foreground-subtle">
          {t('inventoryNote')}
        </p>
      </div>

      {summary && (
        <Alert variant={alertVariant}>
          <AlertTitle>{t('alertImportCompleted')}</AlertTitle>
          <AlertDescription>
            {t('summaryText', {
              total: summary.total ?? 0,
              imported: summary.imported ?? 0,
              replaced: summary.replaced ?? 0,
              already_imported: summary.already_imported ?? 0,
              skipped_conflict: summary.skipped_conflict ?? 0,
              failed: summary.failed ?? 0,
            })}
          </AlertDescription>
        </Alert>
      )}

      {inventory.length === 0 ? (
        <p className="text-foreground-subtle">{t('emptyInventory')}</p>
      ) : (
        <CheckboxGroup
          aria-label={t('ariaInventorySelection')}
          allValues={allRelativeDirs}
          value={selectedDirs}
          onValueChange={setSelectedDirs}
        >
          <ScrollArea className="w-full" orientation="horizontal">
            <div className="min-w-[800px]">
              <Table aria-label={t('captionSourceInventory')}>
                <TableCaption className="sr-only">{t('captionSourceInventory')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox parent aria-label={t('ariaSelectAllInventory')} disabled={importing || !available} />
                    </TableHead>
                    <TableHead>{t('navSkills')}</TableHead>
                    <TableHead>{t('colDirectory')}</TableHead>
                    <TableHead>{t('colSlugOverride')}</TableHead>
                    <TableHead>{t('colStatus')}</TableHead>
                    <TableHead>{t('colDescription')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {inventory.map((e) => {
                    const boundSkill = boundSkillMap.get(e.relative_dir)
                    const itemResult = resultItemMap.get(e.relative_dir)
                    return (
                      <TableRow key={e.relative_dir}>
                        <TableCell>
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
                        </TableCell>
                        <TableCell className="font-mono text-foreground-subtle">{e.relative_dir}</TableCell>
                        <TableCell>
                          <Input
                            inputSize="sm"
                            placeholder={t('phSlugDefault')}
                            aria-label={t('ariaSlugOverrideFor', { name: e.name })}
                            value={slugOverrides[e.relative_dir] || ''}
                            onChange={(ev) => onSlugOverrideChange(e.relative_dir, ev.target.value)}
                            disabled={importing || !available}
                            className="w-32 font-mono text-xs"
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
                                    className="block text-xs underline text-foreground-subtle hover:text-foreground"
                                  >
                                    {t('linkViewSlug', { slug: itemResult.slug })}
                                  </Link>
                                )}
                                {itemResult.message && (
                                  <span className="block text-xs text-foreground-subtle">{itemResult.message}</span>
                                )}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-foreground-subtle">{truncate(e.description)}</TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          </ScrollArea>
        </CheckboxGroup>
      )}
    </div>
  )
}
