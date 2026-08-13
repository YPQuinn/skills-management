import { useCallback, useEffect, useRef, useState } from 'react'
import { importSkills } from './skill-api'
import type { SourceDetail } from './source-api'
import type { ImportResponse, ImportItemResult, ImportSummary, ImportSelector } from './skill-api'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

function recomputeSummary(items: ImportItemResult[]): ImportSummary {
  const summary: ImportSummary = {
    total: items.length,
    imported: 0,
    already_imported: 0,
    skipped_conflict: 0,
    replaced: 0,
    failed: 0,
  }
  for (const item of items) {
    if (item.status === 'imported') summary.imported++
    else if (item.status === 'already_imported') summary.already_imported++
    else if (item.status === 'skipped_conflict') summary.skipped_conflict++
    else if (item.status === 'replaced') summary.replaced++
    else if (item.status === 'failed') summary.failed++
  }
  return summary
}

export function useSourceImport(
  source: SourceDetail | null,
  sourceId: number | null,
  loadSkills: () => void,
) {
  const [selectedDirs, setSelectedDirs] = useState<string[]>([])
  const [slugOverrides, setSlugOverrides] = useState<Record<string, string>>({})
  const [allowLarge, setAllowLarge] = useState(false)
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<unknown | null>(null)
  const [importResult, setImportResult] = useState<ImportResponse | null>(null)

  const [conflictQueue, setConflictQueue] = useState<ImportItemResult[]>([])
  const [replacePending, setReplacePending] = useState(false)
  const [replaceError, setReplaceError] = useState<unknown | null>(null)

  const operationInflight = useRef<AbortController | null>(null)

  const pendingConflict = conflictQueue[0] || null

  const cancelOperation = useCallback(() => {
    operationInflight.current?.abort()
    operationInflight.current = null
  }, [])

  useEffect(() => {
    return () => {
      cancelOperation()
    }
  }, [cancelOperation])

  const resetImportState = useCallback(() => {
    cancelOperation()
    setSelectedDirs([])
    setSlugOverrides({})
    setImportResult(null)
    setImportError(null)
    setConflictQueue([])
    setReplacePending(false)
    setReplaceError(null)
  }, [cancelOperation])

  const handleSlugOverrideChange = (dir: string, value: string) => {
    setSlugOverrides((prev) => ({ ...prev, [dir]: value }))
  }

  const handleImport = async (options: { all?: boolean }) => {
    if (sourceId === null) return
    const dirsToImport = options.all ? source?.inventory.map((e) => e.relative_dir) || [] : selectedDirs
    if (!options.all && dirsToImport.length === 0) return

    cancelOperation()
    const controller = new AbortController()
    operationInflight.current = controller

    setImporting(true)
    setImportError(null)
    try {
      const body = options.all
        ? { source_id: sourceId, all: true, allow_large: allowLarge }
        : {
            source_id: sourceId,
            selectors: dirsToImport.map((d) => {
              const override = (slugOverrides[d] || '').trim()
              return override ? { relative_dir: d, slug: override } : { relative_dir: d }
            }),
            allow_large: allowLarge,
          }

      const res = await importSkills(body, controller.signal)
      if (operationInflight.current !== controller) return

      setImportResult(res)

      const conflicts = res.items.filter((item) => item.status === 'skipped_conflict' && item.replaces)
      setConflictQueue(conflicts)
      setReplaceError(null)

      loadSkills()
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (operationInflight.current === controller) {
        setImportError(err)
      }
    } finally {
      if (operationInflight.current === controller) {
        setImporting(false)
      }
    }
  }

  const handleConfirmReplace = async () => {
    if (!pendingConflict || sourceId === null) return
    const targetItem = pendingConflict

    cancelOperation()
    const controller = new AbortController()
    operationInflight.current = controller

    setReplacePending(true)
    setReplaceError(null)
    try {
      const slug = (targetItem.requested_slug || '').trim()
      const selector: ImportSelector = slug
        ? { relative_dir: targetItem.relative_dir, slug: slug, replace: true }
        : { relative_dir: targetItem.relative_dir, replace: true }

      const body = {
        source_id: sourceId,
        selectors: [selector],
        allow_large: allowLarge,
      }
      const res = await importSkills(body, controller.signal)
      if (operationInflight.current !== controller) return

      const updatedItems = importResult
        ? importResult.items.map((item) =>
            item.relative_dir === targetItem.relative_dir ? (res.items[0] || item) : item,
          )
        : res.items

      const updatedSummary = recomputeSummary(updatedItems)
      setImportResult({ items: updatedItems, summary: updatedSummary })

      setConflictQueue((prev) => prev.slice(1))
      setReplaceError(null)
      loadSkills()
    } catch (err: unknown) {
      if (isAbortError(err)) return
      if (operationInflight.current === controller) {
        setReplaceError(err)
      }
    } finally {
      if (operationInflight.current === controller) {
        setReplacePending(false)
      }
    }
  }

  const handleDeclineReplace = () => {
    setConflictQueue((prev) => prev.slice(1))
    setReplaceError(null)
  }

  return {
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
    cancelOperation,
    resetImportState,
    handleImport,
    handleConfirmReplace,
    handleDeclineReplace,
  }
}
