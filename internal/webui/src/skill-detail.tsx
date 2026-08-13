import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Spinner } from '@appica/ui-react/spinner'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@appica/ui-react/tabs'
import { ArrowLeft } from '@appica/icons-react'
import { fetchSkill } from './skill-api'
import type { Skill } from './skill-api'
import { useLocale } from './locale-context'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillDetailPage() {
  const { slug } = useParams()
  const { t, formatTime, getErrorMessage } = useLocale()
  const [skill, setSkill] = useState<Skill | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    if (!slug) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    setSkill(null)
    setError(null)

    fetchSkill(slug, controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSkill(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(err)
      })

    return controller
  }, [slug])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  if (error && !skill) {
    return (
      <Alert variant="error">
        <AlertTitle>{t('alertCouldNotLoadSkill')}</AlertTitle>
        <AlertDescription>{getErrorMessage(error, 'errSkillNotFound')}</AlertDescription>
      </Alert>
    )
  }

  if (!skill) {
    return <Spinner className="text-3xl text-foreground-subtle" aria-label={t('ariaLoadingSkill')} />
  }

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/skills"
          className="inline-flex items-center gap-1 text-sm text-foreground-subtle underline decoration-border underline-offset-2 hover:decoration-foreground"
        >
          <ArrowLeft className="size-4" />
          {t('linkAllSkills')}
        </Link>
        <div className="flex items-start justify-between gap-4 mt-2">
          <div>
            <h1 className="text-2xl font-bold">{skill.name}</h1>
            <p className="font-mono text-sm text-foreground-subtle">{skill.slug}</p>
          </div>
          {skill.binding ? (
            <Badge variant="success">{t('badgeBound')}</Badge>
          ) : (
            <Badge variant="outline">{t('badgeUnbound')}</Badge>
          )}
        </div>
        <p className="mt-2 text-foreground-subtle">{skill.description}</p>
      </div>

      <Tabs defaultValue="overview" variant="line">
        <TabsList>
          <TabsTrigger value="overview">{t('tabOverview')}</TabsTrigger>
          <TabsTrigger value="synchronization" disabled>
            {t('tabSynchronization')}
          </TabsTrigger>
          <TabsTrigger value="distribution" disabled>
            {t('tabDistribution')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="pt-4 space-y-6">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 text-sm">
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">{t('labelSlug')}</div>
              <div className="font-mono font-medium mt-0.5">{skill.slug}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">{t('labelCreated')}</div>
              <div className="font-medium mt-0.5">{formatTime(skill.created_at)}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">{t('labelUpdated')}</div>
              <div className="font-medium mt-0.5">{formatTime(skill.updated_at)}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">{t('labelStoreDigest')}</div>
              <div className="font-mono text-xs break-all mt-0.5">{skill.store_digest || '—'}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">{t('labelBaselineDigest')}</div>
              <div className="font-mono text-xs break-all mt-0.5">{skill.baseline_digest || '—'}</div>
            </div>
          </div>

          <div className="border border-border rounded-xl p-6 bg-background space-y-4">
            <h2 className="text-lg font-semibold">{t('titleSourceBinding')}</h2>
            {skill.binding ? (
              <div className="grid gap-4 sm:grid-cols-2 text-sm">
                <div>
                  <div className="text-foreground-subtle">{t('labelSourceName')}</div>
                  <div className="font-medium mt-0.5">
                    <Link
                      to={`/sources/${encodeURIComponent(skill.binding.source_name)}`}
                      className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                    >
                      {skill.binding.source_name}
                    </Link>
                  </div>
                </div>
                <div>
                  <div className="text-foreground-subtle">{t('labelRelativeDir')}</div>
                  <div className="font-mono font-medium mt-0.5">{skill.binding.relative_dir}</div>
                </div>
                <div>
                  <div className="text-foreground-subtle">{t('labelSourceCommit')}</div>
                  <div className="font-mono text-xs mt-0.5">{skill.binding.source_commit || '—'}</div>
                </div>
                <div>
                  <div className="text-foreground-subtle">{t('labelImportedAt')}</div>
                  <div className="font-medium mt-0.5">{formatTime(skill.binding.imported_at)}</div>
                </div>
                <div className="sm:col-span-2">
                  <div className="text-foreground-subtle">{t('labelBindingDigest')}</div>
                  <div className="font-mono text-xs break-all mt-0.5">{skill.binding.digest}</div>
                </div>
              </div>
            ) : (
              <p className="text-sm text-foreground-subtle">
                {t('unboundSkillText')}
              </p>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
