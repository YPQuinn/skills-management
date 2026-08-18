import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertIcon, AlertTitle, AlertDescription, AlertAction } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Spinner } from '@appica/ui-react/spinner'
import { DatabaseImport, CircleXFilled } from '@appica/icons-react'
import { useLocale } from './locale-context'
import { ApiError } from './locale-dictionary'

type BootstrapState = 'uninitialized' | 'state_missing' | 'invalid' | 'ready'

export interface StatusResponse {
  state: BootstrapState
  message?: string
}

interface ErrorEnvelope {
  error?: { message?: string }
}

async function postSetup(body: Record<string, unknown>): Promise<void> {
  const res = await fetch('/api/v1/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = (await res.json().catch(() => ({}))) as ErrorEnvelope
  if (!res.ok) {
    throw new ApiError(data?.error?.message, 'errSetupFailed')
  }
}

export function Setup({ status }: { status: StatusResponse }) {
  const navigate = useNavigate()
  const { t, getErrorMessage } = useLocale()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [storePath, setStorePath] = useState('')

  const finishReady = () => navigate('/skills', { replace: true })

  const handleRecover = async () => {
    setLoading(true)
    setError(null)
    try {
      await postSetup({ recover_store: true })
      finishReady()
    } catch (err: unknown) {
      setError(err)
    } finally {
      setLoading(false)
    }
  }

  if (status.state === 'invalid') {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4">
        <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background">
          <Alert variant="error" className="mb-4">
            <AlertIcon>
              <CircleXFilled />
            </AlertIcon>
            <AlertTitle>{t('alertStateError', { state: status.state })}</AlertTitle>
            <AlertDescription>{status.message || t('defaultStateErrorMsg')}</AlertDescription>
          </Alert>
          <p className="text-sm text-foreground-muted">{t('stateErrorHelpText')}</p>
        </div>
      </div>
    )
  }

  if (status.state === 'state_missing') {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4">
        <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background space-y-4">
          <Alert variant="warning">
            <AlertIcon>
              <DatabaseImport />
            </AlertIcon>
            <AlertTitle>{t('alertStateError', { state: status.state })}</AlertTitle>
            <AlertDescription>{status.message || t('defaultStateErrorMsg')}</AlertDescription>
          </Alert>
          <p className="text-sm text-foreground-muted">{t('recoverStoreHelp')}</p>
          <p className="text-sm text-foreground-muted">{t('recoverStoreUnrecoverable')}</p>
          {error !== null && (
            <Alert variant="error">
              <AlertTitle>{t('alertSetupError')}</AlertTitle>
              <AlertDescription>{getErrorMessage(error, 'errRecoverFailed')}</AlertDescription>
            </Alert>
          )}
          <Alert variant="warning">
            <AlertTitle>{t('recoverStoreTitle')}</AlertTitle>
            <AlertDescription>{t('recoverStoreActionHelp')}</AlertDescription>
            <AlertAction>
              <Button
                type="button"
                disabled={loading}
                focusableWhenDisabled
                onClick={() => void handleRecover()}
              >
                {loading && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
                {loading ? t('btnRecoveringStore') : t('btnRecoverStore')}
              </Button>
            </AlertAction>
          </Alert>
        </div>
      </div>
    )
  }

  const handleSetup = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmedPath = storePath.trim()
    if (trimmedPath !== '' && !trimmedPath.startsWith('/')) {
      setError(new ApiError(undefined, 'errAbsoluteMatch'))
      return
    }
    setLoading(true)
    setError(null)
    try {
      await postSetup({ store_path: trimmedPath })
      finishReady()
    } catch (err: unknown) {
      setError(err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-dvh flex items-center justify-center p-4">
      <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background">
        <h1 className="text-xl font-semibold mb-2">{t('welcomeTitle')}</h1>
        <p className="text-foreground-muted mb-6 text-sm">{t('welcomeSubtitle')}</p>
        {error !== null && (
          <Alert variant="error" className="mb-6">
            <AlertTitle>{t('alertSetupError')}</AlertTitle>
            <AlertDescription>{getErrorMessage(error, 'errSetupFailed')}</AlertDescription>
          </Alert>
        )}
        <form onSubmit={handleSetup} className="space-y-4">
          <Field name="storePath">
            <FieldLabel>{t('storePathLabel')}</FieldLabel>
            <Input
              value={storePath}
              onChange={(e) => {
                setStorePath(e.target.value)
                setError(null)
              }}
              placeholder={t('storePathPlaceholder')}
            />
          </Field>
          <Button type="submit" disabled={loading} focusableWhenDisabled className="w-full">
            {loading && <Spinner data-icon="start" currentColor className="text-[1.2em]" />}
            {loading ? t('btnInitializing') : t('btnInitialize')}
          </Button>
        </form>
      </div>
    </div>
  )
}
