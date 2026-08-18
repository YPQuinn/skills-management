import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Spinner } from '@appica/ui-react/spinner'
import { Plus } from '@appica/icons-react'
import { createGroup } from './group-api'
import { useLocale } from './locale-context'

interface CreateGroupFormProps {
  onCreated?: () => void
}

export function CreateGroupForm({ onCreated }: CreateGroupFormProps) {
  const navigate = useNavigate()
  const { t, getErrorMessage } = useLocale()
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown | null>(null)

  const inflight = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return

    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    setLoading(true)
    setError(null)
    try {
      const created = await createGroup(trimmed, controller.signal)
      if (inflight.current !== controller) return

      setName('')
      if (onCreated) {
        onCreated()
      } else {
        navigate(`/groups/${created.id}`)
      }
    } catch (err: unknown) {
      if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
      if (inflight.current === controller) setError(err)
    } finally {
      if (inflight.current === controller) setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div>
        <h2 className="text-lg font-semibold">{t('createGroupTitle')}</h2>
        <p className="text-sm text-foreground-muted">{t('createGroupSubtitle')}</p>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCreateGroupFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errCreatingGroupFailed')}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-col sm:flex-row gap-4 items-end">
        <Field name="name" className="flex-1">
          <FieldLabel>{t('labelGroupName')}</FieldLabel>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              setError(null)
            }}
            placeholder={t('phGroupName')}
            required
          />
        </Field>
        <Button type="submit" disabled={loading || !name.trim()} focusableWhenDisabled>
          {loading ? (
            <Spinner data-icon="start" currentColor className="text-[1.2em]" />
          ) : (
            <Plus data-icon="start" />
          )}
          {loading ? t('btnCreatingGroup') : t('btnCreateGroup')}
        </Button>
      </div>
    </form>
  )
}
