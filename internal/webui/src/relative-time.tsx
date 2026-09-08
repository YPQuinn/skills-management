import { useLocale } from './locale-context'

export function RelativeTime({ value }: { value?: string }) {
  const { formatTime, formatRelativeTime } = useLocale()
  const relative = formatRelativeTime(value)
  if (!value) return relative
  return (
    <time dateTime={value} title={formatTime(value)}>
      {relative}
    </time>
  )
}
