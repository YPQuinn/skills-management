import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Spinner } from '@appica/ui-react/spinner'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogClose,
} from '@appica/ui-react/dialog'
import { Pencil } from '@appica/icons-react'
import { renameSource } from './source-api'
import { useLocale } from './locale-context'
import { useNotifySuccess } from './notify-success'

export function SourceRenameAction({ sourceId, name }: { sourceId: number; name: string }) {
  const { t, getErrorMessage } = useLocale()
  const navigate = useNavigate()
  const notifySuccess = useNotifySuccess()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(name)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<unknown | null>(null)

  const trimmed = draft.trim()
  const canSubmit = trimmed !== '' && trimmed !== name && !pending

  const handleOpenChange = (next: boolean) => {
    if (pending) return
    if (next) {
      setDraft(name)
      setError(null)
    }
    setOpen(next)
  }

  return (
    <>
      <Button variant="outline" onClick={() => handleOpenChange(true)}>
        <Pencil className="size-4" />
        {t('btnRename')}
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:w-110" closeLabel={t('btnClose')}>
          <form
            onSubmit={async (e) => {
              e.preventDefault()
              if (!canSubmit) return
              setPending(true)
              setError(null)
              try {
                const renamed = await renameSource(sourceId, trimmed)
                notifySuccess(t('toastRenamed'))
                setOpen(false)
                navigate(`/sources/${encodeURIComponent(renamed.name)}`)
              } catch (err: unknown) {
                setError(err)
              } finally {
                setPending(false)
              }
            }}
          >
            <DialogHeader>
              <DialogTitle>{t('renameSourceTitle')}</DialogTitle>
              <DialogDescription>{t('renameSourceDesc')}</DialogDescription>
            </DialogHeader>
            <DialogBody className="space-y-4">
              {error !== null && (
                <Alert variant="error">
                  <AlertTitle>{t('alertRenameFailed')}</AlertTitle>
                  <AlertDescription>{getErrorMessage(error, 'errRenamingSourceFailed')}</AlertDescription>
                </Alert>
              )}
              <Field name="name">
                <FieldLabel>{t('labelName')}</FieldLabel>
                <Input
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  autoFocus
                />
              </Field>
            </DialogBody>
            <DialogFooter>
              <DialogClose render={<Button type="button" variant="soft" disabled={pending}>{t('btnCancel')}</Button>} />
              <Button type="submit" disabled={!canSubmit} focusableWhenDisabled>
                {pending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
                {pending ? t('btnRenaming') : t('btnRename')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
