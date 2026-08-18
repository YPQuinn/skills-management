import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import { LinkOff, Trash } from '@appica/icons-react'
import type { Skill } from './skill-api'
import {
  detachSkill,
  rebindSkill,
  fetchSkillDeletion,
  deleteSkill,
  type SkillDeletePreview,
} from './cleanup-api'
import { ResourceDeleteDialog } from './resource-delete-dialog'
import { namesList } from './cleanup-api'
import { RebindDialog } from './skill-rebind-dialog'
import { useLocale } from './locale-context'

export function SkillCleanupActions({
  skill,
  onSkillUpdated,
}: {
  skill: Skill
  onSkillUpdated: (skill: Skill) => void
}) {
  const { t, getErrorMessage } = useLocale()
  const navigate = useNavigate()
  const [detachOpen, setDetachOpen] = useState(false)
  const [rebindOpen, setRebindOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [preview, setPreview] = useState<SkillDeletePreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [warning, setWarning] = useState('')

  const run = async (fn: () => Promise<void>) => {
    setPending(true)
    setError(null)
    try {
      await fn()
    } catch (err: unknown) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      {skill.binding && (
        <Button variant="outline" onClick={() => { setError(null); setDetachOpen(true) }}>
          <LinkOff className="size-4" />
          {t('btnDetach')}
        </Button>
      )}
      <Button variant="outline" onClick={() => { setError(null); setRebindOpen(true) }}>
        {t('btnRebind')}
      </Button>
      <Button
        variant="destructive"
        onClick={() => {
          setError(null)
          setPreview(null)
          setWarning('')
          setDeleteOpen(true)
          setLoading(true)
          fetchSkillDeletion(skill.id)
            .then(setPreview)
            .catch(setError)
            .finally(() => setLoading(false))
        }}
      >
        <Trash className="size-4" />
        {t('btnDelete')}
      </Button>

      <AlertDialog open={detachOpen} onOpenChange={(next) => !next && !pending && setDetachOpen(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('detachSkillTitle', { name: skill.name })}</AlertDialogTitle>
            <AlertDialogDescription>{t('detachSkillDesc')}</AlertDialogDescription>
          </AlertDialogHeader>
          {error !== null && (
            <Alert variant="error">
              <AlertTitle>{t('alertCleanupFailed')}</AlertTitle>
              <AlertDescription>{getErrorMessage(error, 'errCleanupFailed')}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <Button variant="soft" disabled={pending} onClick={() => setDetachOpen(false)}>
              {t('btnCancel')}
            </Button>
            <Button
              variant="destructive"
              disabled={pending}
              focusableWhenDisabled
              onClick={() =>
                run(async () => {
                  const next = await detachSkill(skill.id)
                  onSkillUpdated(next)
                  setDetachOpen(false)
                })
              }
            >
              {pending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
              {pending ? t('btnDetaching') : t('btnDetach')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <RebindDialog
        skill={skill}
        open={rebindOpen}
        pending={pending}
        error={error}
        onClose={() => setRebindOpen(false)}
        onConfirm={(sourceId, dir) =>
          run(async () => {
            const res = await rebindSkill(skill.id, sourceId, dir)
            onSkillUpdated(res.skill)
            setRebindOpen(false)
          })
        }
      />

      <ResourceDeleteDialog
        open={deleteOpen}
        title={t('deleteSkillTitle', { name: skill.name })}
        description={preview?.referenced ? t('deleteSkillReferenced') : t('deleteSkillDesc')}
        details={[
          preview?.groups.length ? t('deleteSkillGroups', { names: namesList(preview.groups) }) : '',
          preview?.targets.length ? t('deleteSkillTargets', { names: namesList(preview.targets) }) : '',
          preview?.links.length
            ? t('deleteSkillLinks', { names: preview.links.map((l) => `${l.slug}@${l.target_name}`).join(', ') })
            : '',
        ].filter(Boolean)}
        loading={loading}
        ready={preview !== null}
        pending={pending}
        error={error}
        warning={warning}
        onClose={() => {
          setDeleteOpen(false)
          if (warning) navigate('/skills')
        }}
        onConfirm={() => {
          if (!preview) return
          run(async () => {
            const res = await deleteSkill(skill.id, preview.referenced)
            const lost = (res.links || []).filter((l) => l.result === 'ownership_lost')
            if (lost.length > 0) {
              setWarning(t('ownershipLostWarning', { names: lost.map((l) => l.slug).join(', ') }))
              return
            }
            navigate('/skills')
          })
        }}
      />
    </div>
  )
}
