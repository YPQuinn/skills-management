import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { SourceStatusBadge } from './source-status'
import type { SourceDetail, SourceSummary } from './source-api'
import { fetchSkills } from './skill-api'
import type { Skill } from './skill-api'
import { SourceInventory } from './source-inventory'
import { SourceReplaceDialog } from './source-replace-dialog'
import { SourceFactsGrid } from './source-facts'
import { SourceIssuesList } from './source-issues-list'
import { useSourceImport } from './use-source-import'
import { useLocale } from './locale-context'
import { ApiError } from './locale-dictionary'

interface ErrorEnvelope {
  error?: { message?: string }
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SourceDetailPage({ onCheckSuccess }: { onCheckSuccess?: () => void }) {
  const { name } = useParams()
  const { t, formatTime, getErrorMessage } = useLocale()
  const [source, setSource] = useState<SourceDetail | null>(null)
  const [sourceId, setSourceId] = useState<number | null>(null)
  const [skills, setSkills] = useState<Skill[]>([])
  const [error, setError] = useState<unknown | null>(null)
  const [checking, setChecking] = useState(false)

  const inflight = useRef<AbortController | null>(null)
  const skillsInflight = useRef<AbortController | null>(null)

  const loadSkills = useCallback(() => {
    skillsInflight.current?.abort()
    const controller = new AbortController()
    skillsInflight.current = controller

    fetchSkills(controller.signal)
      .then((data) => {
        if (skillsInflight.current === controller) setSkills(data)
      })
      .catch(() => {})
  }, [])

  const {
    selectedDirs,
    setSelectedDirs,
    slugOverrides,
    handleSlugOverrideChange,
    allowLarge,
    setAllowLarge,
    importing,
    importError,
    importResult,
    pendingConflict,
    replacePending,
    replaceError,
    resetImportState,
    handleImport,
    handleConfirmReplace,
    handleDeclineReplace,
  } = useSourceImport(source, sourceId, loadSkills)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    fetch('/api/v1/sources', { signal: controller.signal })
      .then(async (res) => {
        if (!res.ok) {
          const data = (await res.json()) as ErrorEnvelope
          throw new ApiError(data?.error?.message, 'errServerResponded', { status: res.status })
        }
        return res.json()
      })
      .then((data: { items: SourceSummary[] }) => {
        const found = data.items.find((s) => s.name === name)
        if (!found) throw new ApiError(undefined, 'errSourceNotFound')
        if (inflight.current === controller) setSourceId(found.id)
        return fetch(`/api/v1/sources/${found.id}`, { signal: controller.signal })
      })
      .then(async (res) => {
        if (!res.ok) {
          const data = (await res.json()) as ErrorEnvelope
          throw new ApiError(data?.error?.message, 'errServerResponded', { status: res.status })
        }
        return res.json()
      })
      .then((data: SourceDetail) => {
        if (inflight.current === controller) setSource(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(err)
      })

    loadSkills()
    return controller
  }, [name, loadSkills])

  useEffect(() => {
    setSource(null)
    setError(null)
    resetImportState()
    load()
    return () => {
      inflight.current?.abort()
      inflight.current = null
      skillsInflight.current?.abort()
      skillsInflight.current = null
    }
  }, [load, resetImportState])

  const check = async () => {
    if (sourceId === null) return
    resetImportState()
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setChecking(true)
    setError(null)
    try {
      const res = await fetch(`/api/v1/sources/${sourceId}/check`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
        signal: controller.signal,
      })
      if (!res.ok) {
        const data = (await res.json()) as ErrorEnvelope
        throw new ApiError(data?.error?.message, 'errCheckingSourceFailed')
      }
      const data = (await res.json()) as SourceDetail
      if (inflight.current === controller) {
        setSource(data)
        onCheckSuccess?.()
      }
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (inflight.current === controller) setError(err)
    } finally {
      if (inflight.current === controller) setChecking(false)
    }
  }

  if (error && !source) {
    return (
      <Alert variant="error">
        <AlertTitle>{t('alertCouldNotLoadSource')}</AlertTitle>
        <AlertDescription>{getErrorMessage(error, 'errSourceNotFound')}</AlertDescription>
      </Alert>
    )
  }
  if (!source) {
    return <Spinner className="text-3xl text-foreground-subtle" aria-label={t('ariaLoadingSource')} />
  }

  const boundSkillMap = new Map<string, Skill>()
  if (sourceId !== null && skills.length > 0) {
    for (const sk of skills) {
      if (sk.binding && sk.binding.source_id === sourceId) {
        boundSkillMap.set(sk.binding.relative_dir, sk)
      }
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <Link
            to="/sources"
            className="text-sm text-foreground-subtle underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {t('linkAllSources')}
          </Link>
          <h1 className="text-2xl font-bold mt-1">{source.name}</h1>
          <p className="text-foreground-subtle text-sm break-all">{source.location}</p>
        </div>
        <div className="flex flex-col items-end gap-2">
          <SourceStatusBadge available={source.available} stale={source.stale} />
          <Button onClick={check} disabled={checking} focusableWhenDisabled>
            {checking && <Spinner data-icon="start" currentColor />}
            {checking ? t('btnChecking') : t('btnCheckAgain')}
          </Button>
        </div>
      </div>

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertCheckFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errCheckingSourceFailed')}</AlertDescription>
        </Alert>
      )}

      {importError !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertImportFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(importError, 'errImportingSkillsFailed')}</AlertDescription>
        </Alert>
      )}

      {!source.available && source.last_error && (
        <Alert variant="error">
          <AlertTitle>{t('alertSourceUnavailable')}</AlertTitle>
          <AlertDescription>
            {t('descSourceUnavailable', {
              error: source.last_error,
              time: formatTime(source.last_successful_check_at),
            })}
          </AlertDescription>
        </Alert>
      )}

      <SourceFactsGrid source={source} />

      <SourceInventory
        inventory={source.inventory}
        available={source.available}
        boundSkillMap={boundSkillMap}
        selectedDirs={selectedDirs}
        setSelectedDirs={setSelectedDirs}
        slugOverrides={slugOverrides}
        onSlugOverrideChange={handleSlugOverrideChange}
        allowLarge={allowLarge}
        setAllowLarge={setAllowLarge}
        importing={importing}
        importResult={importResult}
        onImport={handleImport}
      />

      <SourceIssuesList issues={source.issues} />

      <SourceReplaceDialog
        pendingConflict={pendingConflict}
        replacePending={replacePending}
        replaceError={replaceError}
        onConfirm={handleConfirmReplace}
        onDecline={handleDeclineReplace}
      />
    </div>
  )
}
