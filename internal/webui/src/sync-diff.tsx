import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Chip } from '@appica/ui-react/chip'
import { CopyButton } from '@appica/ui-react/copy-button'
import { Input } from '@appica/ui-react/input'
import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@appica/ui-react/accordion'
import { GitCompare, Search } from '@appica/icons-react'
import { ListPageSkeleton } from './list-page-skeleton'
import { useLocale } from './locale-context'
import type { DictionaryKey } from './locale-dictionary'
import type { DiffEntry, DiffNode, DiffResult } from './sync-api'
import {
  aggregatePathDiffs,
  comparisonsAreEmpty,
  expandedDiffEntries,
  type SideState,
} from './sync-diff-paths'

type ChangeVariant = 'success' | 'error' | 'info' | 'warning'
type ChipVariant = 'outline' | 'primary' | 'destructive' | 'secondary'

const CHANGE_BADGES: Record<string, { key: DictionaryKey; variant: ChangeVariant }> = {
  add: { key: 'changeAdd', variant: 'success' },
  delete: { key: 'changeDelete', variant: 'error' },
  content: { key: 'changeContent', variant: 'info' },
  exec: { key: 'changeExec', variant: 'warning' },
  node_type: { key: 'changeNodeType', variant: 'warning' },
}

const NODE_KIND_KEYS: Record<string, DictionaryKey> = {
  file: 'nodeKindFile',
  dir: 'nodeKindDir',
  symlink: 'nodeKindSymlink',
  fifo: 'nodeKindFifo',
  socket: 'nodeKindSocket',
  block_device: 'nodeKindBlockDevice',
  char_device: 'nodeKindCharDevice',
  other: 'nodeKindOther',
}

const SIDE_STATE_KEY: Record<SideState, DictionaryKey> = {
  unchanged: 'chipUnchanged',
  added: 'chipAdded',
  removed: 'chipRemoved',
  changed: 'chipChanged',
}

const SIDE_STATE_VARIANT: Record<SideState, ChipVariant> = {
  unchanged: 'outline',
  added: 'primary',
  removed: 'destructive',
  changed: 'secondary',
}

const DIFF_SIDE_KEY: Record<'upstream' | 'store' | 'source_store', DictionaryKey> = {
  upstream: 'chipUpstream',
  store: 'chipStore',
  source_store: 'diffSideSourceStore',
}

function UnifiedDiff({ text }: { text: string }) {
  return (
    <pre className="mt-2 overflow-x-auto rounded-md bg-background-subtle p-3 font-mono text-xs leading-5">
      {text.split('\n').map((line, index) => (
        <div
          key={index}
          className={
            line.startsWith('+') && !line.startsWith('+++')
              ? 'text-success-emphasis'
              : line.startsWith('-') && !line.startsWith('---')
                ? 'text-error-emphasis'
                : undefined
          }
        >
          {line === '' ? ' ' : line}
        </div>
      ))}
    </pre>
  )
}

function DiffNodeMeta({
  side,
  node,
  t,
}: {
  side: 'from' | 'to'
  node: DiffNode
  t: (key: DictionaryKey, params?: Record<string, string | number>) => string
}) {
  const kindKey = NODE_KIND_KEYS[node.kind]
  const parts = [kindKey ? t(kindKey) : node.kind]
  if (node.exec) parts.push(t('diffExecutable'))
  if (node.size !== undefined) parts.push(t('diffSizeBytes', { size: node.size }))
  return (
    <div>
      <div className="text-foreground-muted">{t(side === 'from' ? 'labelFrom' : 'labelTo')}</div>
      <div className="font-mono break-all mt-0.5">{parts.join(' · ')}</div>
      {node.digest && <div className="font-mono text-[10px] break-all text-foreground-muted">{node.digest}</div>}
    </div>
  )
}

function DiffEntryView({
  entry,
  heading,
  t,
}: {
  entry: DiffEntry
  heading: string
  t: (key: DictionaryKey, params?: Record<string, string | number>) => string
}) {
  const hasText = entry.text !== undefined && entry.text.unified !== ''
  return (
    <div className="border border-border rounded-lg p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{heading}</span>
        {entry.changes.map((change) => {
          const def = CHANGE_BADGES[change]
          if (!def) {
            return (
              <Badge key={change} variant="outline" size="sm">
                {change}
              </Badge>
            )
          }
          return (
            <Badge key={change} variant={def.variant} size="sm">
              {t(def.key)}
            </Badge>
          )
        })}
      </div>
      {hasText ? (
        <UnifiedDiff text={entry.text!.unified} />
      ) : (
        (entry.from !== undefined || entry.to !== undefined) && (
          <div className="mt-2 grid gap-3 sm:grid-cols-2 text-xs">
            {entry.from ? (
              <DiffNodeMeta side="from" node={entry.from} t={t} />
            ) : (
              <div className="text-foreground-muted text-xs">{t('labelFrom')}: —</div>
            )}
            {entry.to ? (
              <DiffNodeMeta side="to" node={entry.to} t={t} />
            ) : (
              <div className="text-foreground-muted text-xs">{t('labelTo')}: —</div>
            )}
          </div>
        )
      )}
    </div>
  )
}

interface SyncDiffProps {
  diff: DiffResult | null
  loading: boolean
  error: unknown | null
  pathFilter: string
  onPathFilterChange: (value: string) => void
}

export function SyncDiff({ diff, loading, error, pathFilter, onPathFilterChange }: SyncDiffProps) {
  const { t, getErrorMessage } = useLocale()
  const rows = diff ? aggregatePathDiffs(diff.comparisons) : []
  const empty = diff !== null && comparisonsAreEmpty(diff.comparisons)

  return (
    <section aria-labelledby="sync-diff-heading" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="sync-diff-heading" className="text-lg font-semibold inline-flex items-center gap-2">
          <GitCompare className="size-5" />
          {t('diffHeading')}
        </h2>
        <Input
          className="w-full max-w-56"
          value={pathFilter}
          onChange={(event) => onPathFilterChange(event.target.value)}
          onClear={() => onPathFilterChange('')}
          clearable
          startSlot={<Search />}
          placeholder={t('phFilterDiffPath')}
          aria-label={t('ariaFilterDiffPath')}
        />
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertDiffFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errLoadingDiffFailed')}</AlertDescription>
        </Alert>
      )}

      {diff === null ? (
        loading ? (
          <ListPageSkeleton label={t('ariaLoadingDiff')} />
        ) : null
      ) : empty ? (
        <p className="text-sm text-foreground-muted">{t('diffEmptyAll')}</p>
      ) : (
        <Accordion multiple variant="flush">
          {rows.map((row) => (
            <AccordionItem key={row.path} value={row.path}>
              <div className="flex items-center gap-1">
                <AccordionTrigger className="min-w-0 flex-1">
                  <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-start">
                    <span className="font-mono text-sm break-all">{row.path}</span>
                    <Chip render={<span />} variant={SIDE_STATE_VARIANT[row.upstream]} size="sm">
                      {t('chipUpstream')}: {t(SIDE_STATE_KEY[row.upstream])}
                    </Chip>
                    <Chip render={<span />} variant={SIDE_STATE_VARIANT[row.store]} size="sm">
                      {t('chipStore')}: {t(SIDE_STATE_KEY[row.store])}
                    </Chip>
                  </span>
                </AccordionTrigger>
                <CopyButton value={row.path} label={t('btnCopy')} />
              </div>
              <AccordionContent>
                <div className="space-y-3">
                  {expandedDiffEntries(row).map((item) => (
                    <DiffEntryView
                      key={`${item.side}:${item.entry.path}:${item.entry.changes.join(',')}`}
                      entry={item.entry}
                      heading={t(DIFF_SIDE_KEY[item.side])}
                      t={t}
                    />
                  ))}
                </div>
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      )}
    </section>
  )
}
