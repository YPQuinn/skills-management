import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Trash } from '@appica/icons-react'
import { deleteTarget, fetchTargetDeletion, namesList, type TargetDeletePreview } from './cleanup-api'
import { ResourceDeleteDialog } from './resource-delete-dialog'
import { useLocale } from './locale-context'

export function TargetDeleteAction({ targetId, name }: { targetId: number; name: string }) {
  const { t } = useLocale()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [preview, setPreview] = useState<TargetDeletePreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [warning, setWarning] = useState('')

  const assignmentNames = (preview?.assignments || [])
    .map((a) => a.skill?.name || a.group?.name || '')
    .filter(Boolean)

  return (
    <>
      <Button
        variant="destructive"
        onClick={() => {
          setError(null)
          setPreview(null)
          setWarning('')
          setOpen(true)
          setLoading(true)
          fetchTargetDeletion(targetId)
            .then(setPreview)
            .catch(setError)
            .finally(() => setLoading(false))
        }}
      >
        <Trash className="size-4" />
        {t('btnDelete')}
      </Button>
      <ResourceDeleteDialog
        open={open}
        title={t('deleteTargetTitle', { name })}
        description={t('deleteTargetDesc')}
        details={[
          assignmentNames.length ? t('deleteTargetAssignments', { names: namesList(assignmentNames.map((n) => ({ name: n }))) }) : '',
          preview?.links.length
            ? t('deleteTargetLinks', { names: preview.links.map((l) => l.slug).join(', ') })
            : '',
        ].filter(Boolean)}
        loading={loading}
        ready={preview !== null}
        pending={pending}
        error={error}
        warning={warning}
        onClose={() => {
          setOpen(false)
          if (warning) navigate('/targets')
        }}
        onConfirm={async () => {
          if (!preview) return
          setPending(true)
          setError(null)
          try {
            const res = await deleteTarget(targetId)
            const lost = (res.links || []).filter((l) => l.result === 'ownership_lost')
            if (lost.length > 0) {
              setWarning(t('ownershipLostWarning', { names: lost.map((l) => l.slug).join(', ') }))
              return
            }
            navigate('/targets')
          } catch (err: unknown) {
            setError(err)
          } finally {
            setPending(false)
          }
        }}
      />
    </>
  )
}
