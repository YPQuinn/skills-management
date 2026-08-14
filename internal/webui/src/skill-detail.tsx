// The Skill detail page: header plus the Overview / Synchronization /
// Distribution tabs. The active tab lives in the ?tab= query parameter so
// views are deep-linkable and follow browser back/forward.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Spinner } from '@appica/ui-react/spinner'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@appica/ui-react/tabs'
import { ArrowLeft } from '@appica/icons-react'
import { fetchSkill } from './skill-api'
import type { Skill } from './skill-api'
import { SkillOverview } from './skill-overview'
import { SkillSyncTab } from './skill-sync-tab'
import { useLocale } from './locale-context'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillDetailPage() {
  const { slug } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const { t, getErrorMessage } = useLocale()
  const [skill, setSkill] = useState<Skill | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const tab = searchParams.get('tab') === 'synchronization' ? 'synchronization' : 'overview'

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

  const handleTabChange = (value: string) => {
    setSearchParams(value === 'overview' ? {} : { tab: value }, { replace: false })
  }

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

      <Tabs value={tab} onValueChange={handleTabChange} variant="line">
        <TabsList>
          <TabsTrigger value="overview">{t('tabOverview')}</TabsTrigger>
          <TabsTrigger value="synchronization">{t('tabSynchronization')}</TabsTrigger>
          <TabsTrigger value="distribution" disabled>
            {t('tabDistribution')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="pt-4">
          <SkillOverview skill={skill} />
        </TabsContent>

        <TabsContent value="synchronization" className="pt-4">
          <SkillSyncTab skill={skill} onSkillUpdated={setSkill} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
