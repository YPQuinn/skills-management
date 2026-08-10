import { useEffect, useState } from 'react'
import { Routes, Route, useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Spinner } from '@appica/ui-react/spinner'
import { SourcesIndex, SourceExplorer } from './sources'
import { Layout } from './layout'

type BootstrapState = 'uninitialized' | 'state_missing' | 'invalid' | 'ready'

interface StatusResponse {
  state: BootstrapState
  message?: string
}

interface ErrorEnvelope {
  error?: { message?: string }
}

function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

function Setup({ status }: { status: StatusResponse }) {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [storePath, setStorePath] = useState('')

  if (status.state === 'state_missing' || status.state === 'invalid') {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4">
        <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background">
          <Alert variant="error" className="mb-4">
            <AlertTitle>Error: {status.state}</AlertTitle>
            <AlertDescription>{status.message || 'Cannot proceed with setup due to invalid or missing state.'}</AlertDescription>
          </Alert>
          <p className="text-sm text-foreground-subtle">
            Please resolve the issue externally before restarting the application.
          </p>
        </div>
      </div>
    )
  }

  const handleSetup = async (e: React.FormEvent) => {
    e.preventDefault()
    
    const trimmedPath = storePath.trim()
    if (trimmedPath !== '' && !trimmedPath.startsWith('/')) {
      setError('Path must be absolute (e.g. starting with "/")')
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
      if (!res.ok) {
        const data = (await res.json()) as ErrorEnvelope
        throw new Error(data?.error?.message || 'Setup failed')
      }
      navigate('/skills', { replace: true })
    } catch (err: unknown) {
      setError(getErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-dvh flex items-center justify-center p-4">
      <div className="w-full max-w-md p-6 border border-border rounded-xl shadow-sm bg-background">
        <h1 className="text-xl font-semibold mb-2">Welcome to Skill Manager</h1>
        <p className="text-foreground-subtle mb-6 text-sm">Initialize your local Skill Store to get started.</p>
        
        {error && (
          <Alert variant="error" className="mb-6">
            <AlertTitle>Setup Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form onSubmit={handleSetup} className="space-y-4">
          <Field name="storePath">
            <FieldLabel>Store Path (Optional)</FieldLabel>
            <Input 
              value={storePath} 
              onChange={e => {
                setStorePath(e.target.value)
                setError(null)
              }} 
              placeholder="Leave blank for default"
            />
          </Field>
          <Button type="submit" disabled={loading} focusableWhenDisabled className="w-full">
            {loading && <Spinner data-icon="start" currentColor />}
            {loading ? 'Initializing...' : 'Initialize'}
          </Button>
        </form>
      </div>
    </div>
  )
}


function Skills() {
  return (
    <Layout>
      <h1 className="text-2xl font-bold mb-4">Skills</h1>
      <p className="text-foreground-subtle">Your local skill store is ready.</p>
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
  return (
    <Layout>
      <h1 className="text-2xl font-bold mb-4">Groups</h1>
      <p className="text-foreground-subtle">Group management will appear here.</p>
    </Layout>
  )
}

function Targets() {
  return (
    <Layout>
      <h1 className="text-2xl font-bold mb-4">Targets</h1>
      <p className="text-foreground-subtle">Agent target configuration will appear here.</p>
    </Layout>
  )
}

export default function App() {
  const navigate = useNavigate()
  const [checking, setChecking] = useState(true)
  const [status, setStatus] = useState<StatusResponse | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/v1/status')
      .then(async (res) => {
        if (!res.ok) {
          throw new Error(`Server responded with ${res.status}`)
        }
        return res.json()
      })
      .then((data: StatusResponse) => {
        setStatus(data)
        if (data.state === 'uninitialized' || data.state === 'state_missing' || data.state === 'invalid') {
          navigate('/setup', { replace: true })
        } else if (window.location.pathname === '/' || window.location.pathname === '/setup') {
          navigate('/skills', { replace: true })
        }
      })
      .catch((err: unknown) => {
        console.error(err)
        setFetchError(getErrorMessage(err))
      })
      .finally(() => setChecking(false))
  }, [navigate])

  if (checking) {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4 bg-background text-foreground">
        <Spinner className="text-3xl text-foreground-subtle" />
      </div>
    )
  }

  if (fetchError) {
    return (
      <div className="min-h-dvh flex flex-col items-center justify-center p-4 bg-background text-foreground">
        <div className="w-full max-w-md">
          <Alert variant="error">
            <AlertTitle>Server Error</AlertTitle>
            <AlertDescription>Failed to fetch application status: {fetchError}</AlertDescription>
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
      <Route path="*" element={<div className="p-4 bg-background text-foreground min-h-dvh">Not Found</div>} />
    </Routes>
  )
}
