import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import type { SourceDetail, SourceSummary } from './source-api'
import { fetchSkills } from './skill-api'
import type { Skill } from './skill-api'
import { SourceInventory } from './source-inventory'
import { useSourceSync, SourceSyncButton, SourceSyncStatus } from './source-sync'
import { summarizeBoundSync } from './source-sync-summary'
import { SourceReplaceDialog } from './source-replace-dialog'
import { SourceFacts } from './source-facts'
import { SourceIssuesList } from './source-issues-list'
import { SourceDetailHeader } from './source-detail-header'
import { diffInventory, inventoryUnchanged } from './source-inventory-diff'
import { useSourceImport } from './use-source-import'
import { ListPageSkeleton } from './list-page-skeleton'
import { useLocale } from './locale-context'
import { ApiError } from './locale-dictionary'
import { useNotifySuccess } from './notify-success'

interface ErrorEnvelope {
  error?: { message?: string }
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SourceDetailPage() {
  const { name } = useParams()
  const { t, formatTime, getErrorMessage } = useLocale()
  const notifySuccess = useNotifySuccess()
  const [source, setSource] = useState<SourceDetail | null>(null)
  const [sourceId, setSourceId] = useState<number | null>(null)
  const [skills, setSkills] = useState<Skill[]>([])
  const [error, setError] = useState<unknown | null>(null)
  const [rescanning, setRescanning] = useState(false)

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

  const sync = useSourceSync(sourceId, loadSkills)

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

  // A rescan replaces the Inventory and nothing else, so its only visible
  // outcome is the list below plus this tally. It reports Inventory entries
  // only: a Source check does not re-evaluate per-Skill Sync Status.
  const rescan = async () => {
    if (sourceId === null) return
    const before = source?.inventory ?? []
    resetImportState()
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setRescanning(true)
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
        throw new ApiError(data?.error?.message, 'errRescanningSourceFailed')
      }
      const data = (await res.json()) as SourceDetail
      if (inflight.current !== controller) return
      setSource(data)
      const change = diffInventory(before, data.inventory)
      notifySuccess(
        inventoryUnchanged(change)
          ? t('toastInventoryUnchanged')
          : t('toastInventoryChanged', { ...change }),
      )
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (inflight.current === controller) setError(err)
    } finally {
      if (inflight.current === controller) setRescanning(false)
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
    return <ListPageSkeleton label={t('ariaLoadingSource')} />
  }

  const boundSkills =
    sourceId === null ? [] : skills.filter((sk) => sk.binding?.source_id === sourceId)
  const boundSkillMap = new Map<string, Skill>(
    boundSkills.map((sk) => [sk.binding!.relative_dir, sk]),
  )

  return (
    <div className="space-y-6">
      <SourceDetailHeader source={source} sourceId={sourceId} />

      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertRescanFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errRescanningSourceFailed')}</AlertDescription>
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

      <SourceFacts source={source} />

      <SourceInventory
        inventory={source.inventory}
        available={source.available}
        lastScannedAt={source.last_checked_at}
        rescanning={rescanning}
        onRescan={() => void rescan()}
        boundSkillMap={boundSkillMap}
        selectedDirs={selectedDirs}
        setSelectedDirs={setSelectedDirs}
        slugOverrides={slugOverrides}
        onSlugOverrideChange={handleSlugOverrideChange}
        allowLarge={allowLarge}
        setAllowLarge={setAllowLarge}
        importing={importing}
        importResult={importResult}
        onImport={async (options) => {
          const res = await handleImport(options)
          if (res !== null && res.summary.failed === 0 && res.summary.skipped_conflict === 0) {
            notifySuccess(t('toastImported'))
          }
        }}
        syncAction={
          boundSkills.length > 0 ? (
            <SourceSyncButton
              syncing={sync.syncing}
              onRun={sync.run}
              lastEvaluatedAt={summarizeBoundSync(boundSkills).lastEvaluatedAt}
            />
          ) : null
        }
        syncStatus={<SourceSyncStatus boundSkills={boundSkills} error={sync.error} />}
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
