import { lazy, Suspense, useEffect, useState, useRef, type ReactNode } from 'react'
import { Routes, Route, useNavigate, useLocation } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { Layout } from './layout'
import { useLocale } from './locale-context'
import { LocaleProvider } from './locale-provider'
import { ApiError, type DictionaryKey } from './locale-dictionary'
import type { StatusResponse } from './setup'
import { ToastProvider, Toaster } from '@appica/ui-react/toast'
import { TooltipProvider } from '@appica/ui-react/tooltip'
import { NotifySuccessBridge } from './notify-success'

const SkillsPages = lazy(() => import('./skills'))
const SourcesPages = lazy(() => import('./sources'))
const GroupsPages = lazy(() => import('./groups'))
const TargetsPages = lazy(() => import('./targets'))
const Setup = lazy(() => import('./setup'))

function FullPageSpinner({ label }: { label: string }) {
  return (
    <div className="min-h-dvh flex flex-col items-center justify-center p-4 bg-background text-foreground">
      <Spinner className="text-3xl text-foreground-muted" aria-label={label} />
    </div>
  )
}

function PageSpinner({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center py-16">
      <Spinner className="text-3xl text-foreground-muted" aria-label={label} />
    </div>
  )
}

function LazySection({
  fallbackKey,
  children,
}: {
  fallbackKey: DictionaryKey
  children: ReactNode
}) {
  const { t } = useLocale()
  return (
    <Layout>
      <Suspense fallback={<PageSpinner label={t(fallbackKey)} />}>
        {children}
      </Suspense>
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
    return <FullPageSpinner label={t('ariaCheckingStatus')} />
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
      <Route
        path="/setup"
        element={
          <Suspense fallback={<FullPageSpinner label={t('ariaCheckingStatus')} />}>
            <Setup status={status} />
          </Suspense>
        }
      />
      <Route path="/skills/*" element={<LazySection fallbackKey="ariaLoadingSkills"><SkillsPages /></LazySection>} />
      <Route path="/sources/*" element={<LazySection fallbackKey="ariaLoadingSources"><SourcesPages /></LazySection>} />
      <Route path="/groups/*" element={<LazySection fallbackKey="ariaLoadingGroups"><GroupsPages /></LazySection>} />
      <Route path="/targets/*" element={<LazySection fallbackKey="ariaLoadingTargets"><TargetsPages /></LazySection>} />
      <Route path="*" element={<div className="p-4 bg-background text-foreground min-h-dvh">{t('notFound')}</div>} />
    </Routes>
  )
}

export default function App() {
  return (
    <LocaleProvider>
      <TooltipProvider>
        <ToastProvider>
          <NotifySuccessBridge>
            <AppRoutes />
          </NotifySuccessBridge>
          <Toaster />
        </ToastProvider>
      </TooltipProvider>
    </LocaleProvider>
  )
}
