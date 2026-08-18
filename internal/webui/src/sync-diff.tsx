// The three-way path diff of decision 05: baseline→source,
// baseline→store, and source→store, with add/delete/content/exec/node-type
// entries, unified text rendering, binary/size metadata, and a path filter
// that re-requests the diff through the REST path query.
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Input } from '@appica/ui-react/input'
import { Spinner } from '@appica/ui-react/spinner'
import { ArrowRight, GitCompare, Search } from '@appica/icons-react'
import { useLocale } from './locale-context'
import type { DictionaryKey } from './locale-dictionary'
import type { DiffEntry, DiffNode, DiffResult } from './sync-api'

type ChangeVariant = 'success' | 'error' | 'info' | 'warning'

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

function sideLabel(side: string, t: (key: DictionaryKey) => string): string {
  switch (side) {
    case 'baseline':
      return t('sideBaseline')
    case 'source':
      return t('sideSource')
    case 'store':
      return t('sideStore')
    default:
      return side
  }
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

function DiffNodeMeta({ side, node, t }: { side: 'from' | 'to'; node: DiffNode; t: (key: DictionaryKey, params?: Record<string, string | number>) => string }) {
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

function DiffEntryView({ entry, t }: { entry: DiffEntry; t: (key: DictionaryKey, params?: Record<string, string | number>) => string }) {
  const hasText = entry.text !== undefined && entry.text.unified !== ''
  return (
    <div className="border border-border rounded-lg p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm break-all">{entry.path}</span>
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
          <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingDiff')} />
        ) : null
      ) : (
        diff.comparisons.map((comparison) => (
          <div
            key={`${comparison.from}-${comparison.to}`}
            className="border border-border rounded-xl p-4 bg-background space-y-3"
          >
            <h3 className="font-medium text-sm">
              {sideLabel(comparison.from, t)}
              <ArrowRight className="size-3.5 inline mx-1 text-foreground-muted" />
              {sideLabel(comparison.to, t)}
            </h3>
            {comparison.entries.length === 0 ? (
              <p className="text-sm text-foreground-muted">{t('diffEmpty')}</p>
            ) : (
              comparison.entries.map((entry) => <DiffEntryView key={entry.path} entry={entry} t={t} />)
            )}
          </div>
        ))
      )}
    </section>
  )
}
