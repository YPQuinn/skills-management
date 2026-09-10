import { useCallback, useEffect, useState, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { ChevronLeft, Folder } from '@appica/icons-react'
import { fetchGroups, fetchGroup, addGroupMembers, removeGroupMember, type GroupView } from './group-api'
import { fetchSkills, type Skill } from './skill-api'
import { fetchTargetAdapters, type TargetAdapter } from './target-api'
import { GroupMembersSection } from './group-members-section'
import { GroupAssignedTargetsSection } from './group-assigned-targets-section'
import { GroupDeleteAction } from './group-delete-action'
import { useLocale } from './locale-context'

export function GroupDetailPage() {
  const { name: rawName } = useParams()
  const { t, formatTime, getErrorMessage } = useLocale()
  const [group, setGroup] = useState<GroupView | null>(null)
  const [allSkills, setAllSkills] = useState<Skill[]>([])
  const [adapters, setAdapters] = useState<TargetAdapter[]>([])
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
      fetchGroups(controller.signal),
      fetchSkills(controller.signal).catch(() => []),
      fetchTargetAdapters(controller.signal).catch(() => []),
    ])
      .then(async ([groups, skills, adList]) => {
        if (inflight.current !== controller) return
        setAllSkills(skills)
        setAdapters(adList)

        const found = groups.find((g) => g.name === rawName || String(g.id) === rawName)
        if (!found) {
          throw new Error('errGroupNotFound')
        }

        const gView = await fetchGroup(found.id, controller.signal)
        if (inflight.current === controller) {
          setGroup(gView)
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

  const handleAddMember = async (skillIds: number[]) => {
    if (!group || skillIds.length === 0) return
    setSubmitting(true)
    setActionError(null)
    try {
      const updated = await addGroupMembers(group.id, skillIds)
      setGroup(updated)
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setSubmitting(false)
    }
  }

  const handleRemoveMember = async (skillId: number) => {
    if (!group) return
    setSubmitting(true)
    setActionError(null)
    try {
      const updated = await removeGroupMember(group.id, skillId)
      setGroup(updated)
    } catch (err: unknown) {
      setActionError(err)
    } finally {
      setSubmitting(false)
    }
  }

  if (loading && group === null) {
    return (
      <div className="flex justify-center py-12">
        <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingGroup')} />
      </div>
    )
  }

  if (error !== null || group === null) {
    return (
      <div className="space-y-4">
        <Link
          to="/groups"
          className="inline-flex items-center gap-1 text-sm font-medium text-foreground-muted hover:text-foreground"
        >
          <ChevronLeft className="size-4" />
          {t('linkAllGroups')}
        </Link>
        <h1 className="text-2xl font-bold">{t('groupsTitle')}</h1>
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadGroup')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errGroupNotFound')}</AlertDescription>
        </Alert>
      </div>
    )
  }

  const memberSet = new Set(group.members.map((m) => m.id))
  const availableSkills = allSkills.filter((s) => !memberSet.has(s.id))

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/groups"
          className="inline-flex items-center gap-1 text-sm font-medium text-foreground-muted hover:text-foreground mb-2"
        >
          <ChevronLeft className="size-4" />
          {t('linkAllGroups')}
        </Link>
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-center gap-3">
            <Folder className="size-7 text-foreground-muted" />
            <h1 className="text-2xl font-bold">{group.name || t('groupsTitle')}</h1>
          </div>
          <GroupDeleteAction groupId={group.id} name={group.name} />
        </div>
        <div className="flex flex-wrap gap-4 text-xs text-foreground-muted mt-2">
          <span>{t('labelCreated')}: {formatTime(group.created_at)}</span>
          <span>{t('labelUpdated')}: {formatTime(group.updated_at)}</span>
        </div>
      </div>

      {actionError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCouldNotLoadGroup')}</AlertTitle>
          <AlertDescription>{getErrorMessage(actionError, 'unknownError')}</AlertDescription>
        </Alert>
      )}

      <GroupMembersSection
        members={group.members}
        availableSkills={availableSkills}
        submitting={submitting}
        onAddMember={handleAddMember}
        onRemoveMember={handleRemoveMember}
      />

      <GroupAssignedTargetsSection targets={group.targets} adapters={adapters} />
    </div>
  )
}
