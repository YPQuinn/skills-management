import type { ComponentType, ReactNode } from 'react'

export type StatusTone = 'muted' | 'info' | 'warning' | 'error'
export type StatusSize = 'sm' | 'md'
export type StatusIcon = ComponentType<{ size?: number | string }>

const TONE: Record<StatusTone, string> = {
  muted: 'text-foreground-muted',
  info: 'text-info-emphasis',
  warning: 'text-warning-emphasis',
  error: 'text-error-emphasis',
}

export function StatusLabel({
  icon: Icon,
  tone,
  size = 'sm',
  className,
  children,
}: {
  icon: StatusIcon
  tone: StatusTone
  size?: StatusSize
  className?: string
  children: ReactNode
}) {
  return (
    <span
      className={[
        'inline-flex shrink-0 items-center gap-1 whitespace-nowrap',
        size === 'md' ? 'text-sm' : 'text-xs',
        TONE[tone],
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <Icon size={size === 'md' ? 14 : 12} />
      {children}
    </span>
  )
}
