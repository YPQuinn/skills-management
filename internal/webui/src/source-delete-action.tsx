import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Trash } from '@appica/icons-react'
import { deleteSource, fetchSourceDeletion, namesList, type SourceDeletePreview } from './cleanup-api'
import { ResourceDeleteDialog } from './resource-delete-dialog'
import { useLocale } from './locale-context'

export function SourceDeleteAction({ sourceId, name }: { sourceId: number; name: string }) {
  const { t } = useLocale()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [preview, setPreview] = useState<SourceDeletePreview | null>(null)
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
          fetchSourceDeletion(sourceId)
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
        title={t('deleteSourceTitle', { name })}
        description={t('deleteSourceDesc')}
        details={
          preview
            ? [
                preview.bound_skills.length
                  ? t('deleteSourceBound', { names: namesList(preview.bound_skills) })
                  : t('deleteSourceNone'),
              ]
            : []
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
            await deleteSource(sourceId, preview.requires_detach)
            navigate('/sources')
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
