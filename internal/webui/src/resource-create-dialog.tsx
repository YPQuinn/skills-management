import { Button } from '@appica/ui-react/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@appica/ui-react/dialog'
import { Plus } from '@appica/icons-react'
import { useLocale } from './locale-context'

export function ResourceCreateDialog({
  open,
  onOpenChange,
  title,
  description,
  triggerLabel,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  triggerLabel: string
  children: React.ReactNode
}) {
  const { t } = useLocale()
  return (
    <>
      <Button onClick={() => onOpenChange(true)}>
        <Plus data-icon="start" />
        {triggerLabel}
      </Button>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-2xl" closeLabel={t('btnClose')}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <DialogBody className="max-h-[min(70vh,36rem)] overflow-y-auto">
            {/* Unmount the form when closed so the next open starts empty. */}
            {open ? children : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  )
}
