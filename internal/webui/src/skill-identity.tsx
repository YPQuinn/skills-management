import type { ReactNode } from 'react'

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
