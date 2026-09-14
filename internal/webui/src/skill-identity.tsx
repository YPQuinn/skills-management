import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Thumbnail } from '@appica/ui-react/thumbnail'
import { Wand } from '@appica/icons-react'

export interface SkillCardSource {
  label: string
  href?: string
}

function distinctSlug(name: string, slug: string): string | undefined {
  return slug !== name ? slug : undefined
}

export function skillLabel(name: string, slug: string): string {
  const extra = distinctSlug(name, slug)
  return extra ? `${name} (${extra})` : name
}

export function SkillIdentity({
  name,
  slug,
  nameNode,
}: {
  name: string
  slug: string
  nameNode?: ReactNode
}) {
  const extra = distinctSlug(name, slug)
  return (
    <span className="flex min-w-0 flex-col">
      {nameNode ?? <span className="truncate font-medium text-foreground-strong">{name}</span>}
      {extra ? <span className="truncate font-mono text-xs text-foreground-muted">{extra}</span> : null}
    </span>
  )
}

export function SkillCardIdentity({
  name,
  slug,
  sources,
  trailing,
}: {
  name: string
  slug: string
  sources?: SkillCardSource[]
  trailing?: ReactNode
}) {
  return (
    <span className="flex min-w-0 flex-1 items-center gap-2.5">
      <Thumbnail variant="icon-soft" size="sm" shape="rounded" aria-hidden>
        <Wand />
      </Thumbnail>
      <span className="flex min-w-0 flex-1 flex-wrap items-baseline">
        <Link
          to={`/skills/${encodeURIComponent(slug)}`}
          className="truncate text-sm font-semibold text-foreground-strong hover:underline underline-offset-2"
        >
          {name}
        </Link>
        {sources?.map((source) => (
          <span key={`${source.href ?? ''}:${source.label}`} className="contents">
            <span className="mx-1.5 shrink-0 text-sm text-foreground-muted" aria-hidden>
              ·
            </span>
            {source.href ? (
              <Link
                to={source.href}
                className="shrink-0 text-sm font-normal text-foreground-muted hover:underline underline-offset-2"
              >
                {source.label}
              </Link>
            ) : (
              <span className="shrink-0 text-sm font-normal text-foreground-muted">{source.label}</span>
            )}
          </span>
        ))}
        {trailing}
      </span>
    </span>
  )
}
