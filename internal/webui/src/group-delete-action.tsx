import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Trash } from '@appica/icons-react'
import { deleteGroup, fetchGroupDeletion, namesList, type GroupDeletePreview } from './cleanup-api'
import { ResourceDeleteDialog } from './resource-delete-dialog'
import { useLocale } from './locale-context'

export function GroupDeleteAction({ groupId, name }: { groupId: number; name: string }) {
  const { t } = useLocale()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [preview, setPreview] = useState<GroupDeletePreview | null>(null)
  const [loading, setLoading] = useState(false)

  return (
    <>
      <Button
        variant="outline"
        className="text-error-emphasis"
        onClick={() => {
          setError(null)
          setPreview(null)
          setOpen(true)
          setLoading(true)
          fetchGroupDeletion(groupId)
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
        title={t('deleteGroupTitle', { name })}
        description={t('deleteGroupDesc')}
        details={
          preview?.targets.length ? [t('deleteGroupAssigned', { names: namesList(preview.targets) })] : []
        }
        loading={loading}
        ready={preview !== null}
        pending={pending}
        error={error}
        onClose={() => setOpen(false)}
        onConfirm={async () => {
          if (!preview) return
          setPending(true)
          setError(null)
          try {
            await deleteGroup(groupId, preview.assigned)
            navigate('/groups')
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
