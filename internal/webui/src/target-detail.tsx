import { useCallback, useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Spinner } from '@appica/ui-react/spinner'
import { ChevronLeft, Target } from '@appica/icons-react'
import {
  fetchTargets,
  fetchTarget,
  createAssignment,
  deleteAssignment,
  type TargetView,
} from './target-api'
import { fetchSkills, type Skill } from './skill-api'
import { fetchGroups, type GroupSummary } from './group-api'
import { TargetAssignmentsSection } from './target-assignments-section'
import { TargetDesiredSetSection } from './target-desired-set-section'
import { TargetDistributionSection } from './target-distribution-section'
import type { DistributionStatus } from './distribution-api'
import { TargetDeleteAction } from './target-delete-action'
import { useLocale } from './locale-context'

export function TargetDetailPage() {
  const { name: rawName } = useParams()
  const { t, formatTime, getErrorMessage } = useLocale()
  const [target, setTarget] = useState<TargetView | null>(null)
  const [allSkills, setAllSkills] = useState<Skill[]>([])
  const [allGroups, setAllGroups] = useState<GroupSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown | null>(null)
  const [actionError, setActionError] = useState<unknown | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    if (!rawName) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)

    Promise.all([
      fetchTargets(controller.signal),
      fetchSkills(controller.signal).catch(() => []),
      fetchGroups(controller.signal).catch(() => []),
    ])
      .then(async ([targets, skills, groups]) => {
        if (inflight.current !== controller) return
        setAllSkills(skills)
        setAllGroups(groups)

        const found = targets.find((t) => t.name === rawName || String(t.id) === rawName)
        if (!found) {
          throw new Error('errTargetNotFound')
        }

        const tView = await fetchTarget(found.id, controller.signal)
        if (inflight.current === controller) {
          setTarget(tView)
        }
      })
      .catch((err: unknown) => {
        if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
        if (inflight.current === controller) setError(err)
      })
      .finally(() => {
        if (inflight.current === controller) setLoading(false)
      })
  }, [rawName])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  const handleAddAssignment = async (kind: 'skill' | 'group', subjectId: number) => {
    if (!target) return
    setSubmitting(true)
    setActionError(null)
    try {
      if (kind === 'skill') {
        await createAssignment(target.id, { kind: 'skill', skill_id: subjectId })
      } else {
        await createAssignment(target.id, { kind: 'group', group_id: subjectId })
      }
      const updated = await fetchTarget(target.id)
      setTarget(updated)
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDeleteAssignment = async (assignmentId: number) => {
    if (!target) return
    setSubmitting(true)
    setActionError(null)
    try {
      const updated = await deleteAssignment(target.id, assignmentId)
      setTarget(updated)
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setSubmitting(false)
    }
  }

  if (loading && target === null) {
    return (
      <div className="flex justify-center py-12">
        <Spinner className="text-3xl text-foreground-subtle" aria-label={t('ariaLoadingTarget')} />
      </div>
    )
  }

  if (error !== null || target === null) {
    return (
      <div className="space-y-4">
        <Link
          to="/targets"
          className="inline-flex items-center gap-1 text-sm font-medium text-foreground-subtle hover:text-foreground"
        >
          <ChevronLeft className="size-4" />
          {t('linkAllTargets')}
        </Link>
        <h1 className="text-2xl font-bold">{t('targetsTitle')}</h1>
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadTarget')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errTargetNotFound')}</AlertDescription>
        </Alert>
      </div>
    )
  }

  const directSkills = target.direct_skills || []
  const groupAssignments = target.groups || []
  const desiredSkills = target.desired_skills || []

  const directSkillIds = new Set(directSkills.map((a) => a.skill?.id).filter((id): id is number => id !== undefined))
  const assignedGroupIds = new Set(groupAssignments.map((a) => a.group?.id).filter((id): id is number => id !== undefined))

  const availableSkills = allSkills.filter((s) => !directSkillIds.has(s.id))
  const availableGroups = allGroups.filter((g) => !assignedGroupIds.has(g.id))

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/targets"
          className="inline-flex items-center gap-1 text-sm font-medium text-foreground-subtle hover:text-foreground mb-2"
        >
          <ChevronLeft className="size-4" />
          {t('linkAllTargets')}
        </Link>
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-center gap-3">
            <Target className="size-7 text-foreground-subtle" />
            <h1 className="text-2xl font-bold">{target.name || t('targetsTitle')}</h1>
            <Badge variant="soft" className="uppercase font-mono">
              {target.adapter}
            </Badge>
          </div>
          <TargetDeleteAction targetId={target.id} name={target.name} />
        </div>
        <div className="flex flex-wrap gap-4 text-xs text-foreground-subtle mt-2">
          <span>{t('colScope')}: <strong className="text-foreground font-mono">{target.scope}</strong></span>
          <span>{t('colPath')}: <strong className="text-foreground font-mono">{target.path}</strong></span>
          {target.project_root && (
            <span>{t('labelProjectRootFact')}: <strong className="text-foreground font-mono">{target.project_root}</strong></span>
          )}
          <span>{t('labelCreated')}: {formatTime(target.created_at)}</span>
        </div>
      </div>

      {actionError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadTarget')}</AlertTitle>
          <AlertDescription>{getErrorMessage(actionError, 'unknownError')}</AlertDescription>
        </Alert>
      )}

      <TargetAssignmentsSection
        directSkills={directSkills}
        groupAssignments={groupAssignments}
        availableSkills={availableSkills}
        availableGroups={availableGroups}
        submitting={submitting}
        onAddAssignment={handleAddAssignment}
        onDeleteAssignment={handleDeleteAssignment}
      />

      <TargetDesiredSetSection desiredSkills={desiredSkills} />

      <TargetDistributionSection
        key={target.id}
        targetId={target.id}
        initial={(target.distribution ?? null) as DistributionStatus | null}
        onChanged={(status) => setTarget((prev) => (prev ? { ...prev, distribution: status } : prev))}
      />
    </div>
  )
}
