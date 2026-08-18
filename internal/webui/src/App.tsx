import { useEffect, useState, useRef } from 'react'
import { Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { SourcesIndex, SourceExplorer } from './sources'
import { SkillsIndex, SkillExplorer } from './skills'
import { GroupsIndex, GroupExplorer } from './groups'
import { TargetsIndex, TargetExplorer } from './targets'
import { Layout } from './layout'
import { useLocale } from './locale-context'
import { LocaleProvider } from './locale-provider'
import { ApiError } from './locale-dictionary'
import { Setup, type StatusResponse } from './setup'

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
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<GroupsIndex />} />
        <Route path="/:name" element={<GroupExplorer />} />
      </Routes>
    </Layout>
  )
}

function Targets() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<TargetsIndex />} />
        <Route path="/:name" element={<TargetExplorer />} />
      </Routes>
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
        <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaCheckingStatus')} />
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
