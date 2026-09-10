import { useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { ListPageSkeleton } from './list-page-skeleton'
import { Unlink } from '@appica/icons-react'
import { fetchSkills } from './skill-api'
import { StatusLabel } from './status-label'
import type { Skill } from './skill-api'
import { SyncStatusBadge } from './sync-status'
import { SkillIdentity } from './skill-identity'
import { useLocale } from './locale-context'

export function SkillListPane({ refreshCounter = 0 }: { refreshCounter?: number }) {
  const { slug } = useParams()
  const { t, getErrorMessage } = useLocale()
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [loading, setLoading] = useState(true)

  const inflight = useRef<AbortController | null>(null)

  useEffect(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)
    fetchSkills(controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSkills(data)
      })
      .catch((err: unknown) => {
        if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
        if (inflight.current === controller) setError(err)
      })
      .finally(() => {
        if (inflight.current === controller) setLoading(false)
      })
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [refreshCounter])

  return (
    <nav aria-label={t('ariaSkillList')} className="rounded-xl border border-border bg-background p-4">
      <h2 className="mb-3 text-sm font-semibold text-foreground-strong">{t('skillsTitle')}</h2>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadSkills')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {loading && skills === null ? (
        <ListPageSkeleton label={t('ariaLoadingSkills')} />
      ) : skills === null ? null : skills.length === 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-foreground-muted">{t('emptySkillsListPane')}</p>
          <Link
            to="/sources"
            className="text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {t('linkImportFromSource')}
          </Link>
        </div>
      ) : (
        <ul className="space-y-1">
          {skills.map((s) => {
            const selected = s.slug === slug
            return (
              <li key={s.id} className="min-w-0">
                <Link
                  to={`/skills/${encodeURIComponent(s.slug)}`}
                  aria-current={selected ? 'page' : undefined}
                  className={`block min-w-0 rounded-lg px-3 py-2 hover:bg-foreground/5 ${
                    selected ? 'bg-foreground/10' : ''
                  }`}
                >
                  <span className="flex items-center justify-between gap-2">
                    <SkillIdentity name={s.name} slug={s.slug} />
                    {s.binding ? (
                      <Badge variant="soft" className="shrink-0 text-xs">
                        {s.binding.source_name}
                      </Badge>
                    ) : (
                      <StatusLabel icon={Unlink} tone="muted">
                        {t('badgeUnbound')}
                      </StatusLabel>
                    )}
                  </span>
                  <span className="mt-0.5 flex justify-end">
                    <SyncStatusBadge status={s.sync_status} />
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </nav>
  )
}
