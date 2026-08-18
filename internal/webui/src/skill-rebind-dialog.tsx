import { useEffect, useState } from 'react'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Spinner } from '@appica/ui-react/spinner'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogBody,
  AlertDialogFooter,
} from '@appica/ui-react/alert-dialog'
import type { Skill } from './skill-api'
import { fetchSources } from './source-api'
import type { SourceDetail, SourceSummary } from './source-api'
import { useLocale } from './locale-context'
import { ApiError } from './locale-dictionary'

export function RebindDialog({
  skill,
  open,
  pending,
  error,
  onClose,
  onConfirm,
}: {
  skill: Skill
  open: boolean
  pending: boolean
  error: unknown | null
  onClose: () => void
  onConfirm: (sourceId: number, relativeDir: string) => void
}) {
  const { t, getErrorMessage } = useLocale()
  const [sources, setSources] = useState<SourceSummary[]>([])
  const [sourceId, setSourceId] = useState('')
  const [detail, setDetail] = useState<SourceDetail | null>(null)
  const [entry, setEntry] = useState('')

  useEffect(() => {
    if (!open) return
    setSourceId('')
    setDetail(null)
    setEntry('')
    fetchSources()
      .then(setSources)
      .catch(() => setSources([]))
  }, [open])

  useEffect(() => {
    if (!sourceId) {
      setDetail(null)
      setEntry('')
      return
    }
    fetch(`/api/v1/sources/${sourceId}`)
      .then(async (res) => {
        if (!res.ok) throw new ApiError(undefined, 'errSourceNotFound')
        return (await res.json()) as SourceDetail
      })
      .then((data) => {
        setDetail(data)
        setEntry('')
      })
      .catch(() => setDetail(null))
  }, [sourceId])

  const selected = detail?.inventory.find((e) => e.relative_dir === entry)
  const identical = Boolean(selected && selected.digest && selected.digest === skill.store_digest)

  return (
    <AlertDialog open={open} onOpenChange={(next) => !next && !pending && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('rebindSkillTitle', { name: skill.name })}</AlertDialogTitle>
          <AlertDialogDescription>{t('rebindSkillDesc')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogBody className="space-y-4">
          <Field name="source">
            <FieldLabel>{t('rebindSelectSource')}</FieldLabel>
            <Select value={sourceId} onValueChange={(v) => setSourceId(String(v))}>
              <SelectTrigger aria-label={t('rebindSelectSource')}>
                <SelectValue placeholder={t('phSelectSource')} />
              </SelectTrigger>
              <SelectContent>
                {sources.map((s) => (
                  <SelectItem key={s.id} value={String(s.id)}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field name="entry">
            <FieldLabel>{t('rebindSelectEntry')}</FieldLabel>
            <Select value={entry} onValueChange={(v) => setEntry(String(v))} disabled={!detail}>
              <SelectTrigger aria-label={t('rebindSelectEntry')}>
                <SelectValue placeholder={t('phSelectEntry')} />
              </SelectTrigger>
              <SelectContent>
                {(detail?.inventory || []).map((e) => (
                  <SelectItem key={e.relative_dir} value={e.relative_dir}>
                    {e.name} ({e.relative_dir})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {selected && (
            <p className="text-sm text-foreground-muted">{identical ? t('rebindIdentical') : t('rebindConflict')}</p>
          )}
          {error !== null && (
            <Alert variant="error">
              <AlertTitle>{t('alertCleanupFailed')}</AlertTitle>
              <AlertDescription>{getErrorMessage(error, 'errCleanupFailed')}</AlertDescription>
            </Alert>
          )}
        </AlertDialogBody>
        <AlertDialogFooter>
          <Button variant="soft" disabled={pending} onClick={onClose}>
            {t('btnCancel')}
          </Button>
          <Button
            disabled={pending || !sourceId || !entry}
            focusableWhenDisabled
            onClick={() => onConfirm(Number(sourceId), entry)}
          >
            {pending && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
            {pending ? t('btnRebinding') : t('btnRebind')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
