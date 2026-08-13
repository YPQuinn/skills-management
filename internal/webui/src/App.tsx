import { useEffect, useState, useRef } from 'react'
import { Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Spinner } from '@appica/ui-react/spinner'
import { SourcesIndex, SourceExplorer } from './sources'
import { SkillsIndex, SkillExplorer } from './skills'
import { Layout } from './layout'
import { useLocale } from './locale-context'
import { LocaleProvider } from './locale-provider'
import { ApiError } from './locale-dictionary'

type BootstrapState = 'uninitialized' | 'state_missing' | 'invalid' | 'ready'

interface StatusResponse {
  state: BootstrapState
  message?: string
}

interface ErrorEnvelope {
  error?: { message?: string }
}

function Setup({ status }: { status: StatusResponse }) {
  const navigate = useNavigate()
  const { t, getErrorMessage } = useLocale()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown | null>(null)
  const [storePath, setStorePath] = useState('')

  if (status.state === 'state_missing' || status.state === 'invalid') {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4">
        <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background">
          <Alert variant="error" className="mb-4">
            <AlertTitle>{t('alertStateError', { state: status.state })}</AlertTitle>
            <AlertDescription>{status.message || t('defaultStateErrorMsg')}</AlertDescription>
          </Alert>
          <p className="text-sm text-foreground-subtle">
            {t('stateErrorHelpText')}
          </p>
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
      const res = await fetch('/api/v1/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ store_path: trimmedPath }),
      })
      const data = (await res.json().catch(() => ({}))) as ErrorEnvelope
      if (!res.ok) {
        throw new ApiError(data?.error?.message, 'errSetupFailed')
      }
      navigate('/skills', { replace: true })
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
        <p className="text-foreground-subtle mb-6 text-sm">{t('welcomeSubtitle')}</p>
        
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
              onChange={e => {
                setStorePath(e.target.value)
                setError(null)
              }} 
              placeholder={t('storePathPlaceholder')}
            />
          </Field>
          <Button type="submit" disabled={loading} focusableWhenDisabled className="w-full">
            {loading && <Spinner data-icon="start" currentColor />}
            {loading ? t('btnInitializing') : t('btnInitialize')}
          </Button>
        </form>
      </div>
    </div>
  )
}

function Skills() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<SkillsIndex />} />
        <Route path="/:slug" element={<SkillExplorer />} />
      </Routes>
    </Layout>
  )
}

function Sources() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<SourcesIndex />} />
        <Route path="/:name" element={<SourceExplorer />} />
      </Routes>
    </Layout>
  )
}

function Groups() {
  const { t } = useLocale()
  return (
    <Layout>
      <h1 className="text-2xl font-bold mb-4">{t('groupsTitle')}</h1>
      <p className="text-foreground-subtle">{t('groupsText')}</p>
    </Layout>
  )
}

function Targets() {
  const { t } = useLocale()
  return (
    <Layout>
      <h1 className="text-2xl font-bold mb-4">{t('targetsTitle')}</h1>
      <p className="text-foreground-subtle">{t('targetsText')}</p>
    </Layout>
  )
}

function AppRoutes() {
  const navigate = useNavigate()
  const location = useLocation()
  const { t, getErrorMessage } = useLocale()
  const [checking, setChecking] = useState(true)
  const [status, setStatus] = useState<StatusResponse | null>(null)
  const [fetchError, setFetchError] = useState<unknown | null>(null)

  const fetchedRef = useRef(false)

  useEffect(() => {
    if (fetchedRef.current) return
    fetchedRef.current = true

    fetch('/api/v1/status')
      .then(async (res) => {
        const data = await res.json().catch(() => ({}))
        if (!res.ok) {
          throw new ApiError(data?.error?.message, 'errServerResponded', { status: res.status })
        }
        return data as StatusResponse
      })
      .then((data: StatusResponse) => {
        setStatus(data)
        if (data.state === 'uninitialized' || data.state === 'state_missing' || data.state === 'invalid') {
          navigate('/setup', { replace: true })
        } else if (location.pathname === '/' || location.pathname === '/setup') {
          navigate('/skills', { replace: true })
        }
      })
      .catch((err: unknown) => {
        console.error(err)
        setFetchError(err)
      })
      .finally(() => setChecking(false))
  }, [navigate, location.pathname])

  if (checking) {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4 bg-background text-foreground">
        <Spinner className="text-3xl text-foreground-subtle" aria-label={t('ariaCheckingStatus')} />
      </div>
    )
  }

  if (fetchError !== null) {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4 bg-background text-foreground">
        <div className="w-full max-w-md">
          <Alert variant="error">
            <AlertTitle>{t('alertServerError')}</AlertTitle>
            <AlertDescription>{t('failedFetchStatus', { error: getErrorMessage(fetchError, 'errServerResponded') })}</AlertDescription>
          </Alert>
        </div>
      </div>
    )
  }

  if (!status) return null

  return (
    <Routes>
      <Route path="/setup" element={<Setup status={status} />} />
      <Route path="/skills/*" element={<Skills />} />
      <Route path="/sources/*" element={<Sources />} />
      <Route path="/groups/*" element={<Groups />} />
      <Route path="/targets/*" element={<Targets />} />
      <Route path="*" element={<div className="p-4 bg-background text-foreground min-h-dvh">{t('notFound')}</div>} />
    </Routes>
  )
}

export default function App() {
  return (
    <LocaleProvider>
      <AppRoutes />
    </LocaleProvider>
  )
}
