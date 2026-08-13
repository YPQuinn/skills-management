import { useState } from 'react'
import { Button } from '@appica/ui-react/button'
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogClose,
} from '@appica/ui-react/dialog'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Input } from '@appica/ui-react/input'
import { useLocale } from './locale-context'

interface SlugOverrideDialogProps {
  name: string
  relativeDir: string
  value: string
  disabled: boolean
  onSave: (value: string) => void
}

export function SlugOverrideDialog({ name, relativeDir, value, disabled, onSave }: SlugOverrideDialogProps) {
  const { t } = useLocale()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(value)

  const handleOpenChange = (nextOpen: boolean) => {
    // Reset the draft to the saved value whenever the dialog opens, so an
    // edit abandoned via Cancel or Escape is never carried into the next open.
    if (nextOpen) setDraft(value)
    setOpen(nextOpen)
  }

  const save = () => {
    onSave(draft)
    setOpen(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled}
            aria-label={t('ariaSlugOverrideFor', { name, dir: relativeDir })}
            className="font-mono text-xs"
          >
            {value || <span className="font-normal text-foreground-subtle">{t('phSlugDefault')}</span>}
          </Button>
        }
      />
      <DialogContent className="sm:w-110" closeLabel={t('btnClose')}>
        <DialogHeader>
          <DialogTitle>{t('slugOverrideDialogTitle')}</DialogTitle>
          <DialogDescription>{t('slugOverrideDialogDesc', { name, dir: relativeDir })}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <Field>
            <FieldLabel>{t('labelSlug')}</FieldLabel>
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('phSlugDefault')}
              className="font-mono text-xs"
            />
          </Field>
        </DialogBody>
        <DialogFooter>
          <DialogClose render={<Button variant="soft">{t('btnCancel')}</Button>} />
          <Button onClick={save}>{t('btnSave')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
