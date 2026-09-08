import type { ReactNode } from 'react'
import { CopyButton } from '@appica/ui-react/copy-button'
import { useLocale } from './locale-context'

export function CopyablePath({ value, className }: { value: string; className?: string }) {
  const { t } = useLocale()
  return (
    <span className={`inline-flex min-w-0 max-w-full items-center gap-1 ${className ?? ''}`}>
      <span className="min-w-0 truncate font-mono">{value}</span>
      <CopyButton value={value} label={t('btnCopy')} />
    </span>
  )
}

export function CopyableField({ label, value }: { label: ReactNode; value?: string }) {
  const { t } = useLocale()
  return (
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0">
        <div className="text-foreground-muted">{label}</div>
        <div className="font-mono text-xs break-all mt-0.5">{value || '—'}</div>
      </div>
      {value ? <CopyButton value={value} label={t('btnCopy')} /> : null}
    </div>
  )
}
