import type { DiffComparison, DiffEntry } from './sync-api'

export type SideState = 'unchanged' | 'added' | 'removed' | 'changed'

export type DiffSide = 'upstream' | 'store' | 'source_store'

export interface PathDiff {
  path: string
  upstream: SideState
  store: SideState
  entries: { side: DiffSide; entry: DiffEntry }[]
}

function changesToState(changes: string[]): SideState {
  if (changes.includes('add')) return 'added'
  if (changes.includes('delete')) return 'removed'
  if (changes.length > 0) return 'changed'
  return 'unchanged'
}

function comparisonSide(from: string, to: string): DiffSide | null {
  if (from === 'baseline' && to === 'source') return 'upstream'
  if (from === 'baseline' && to === 'store') return 'store'
  if (from === 'source' && to === 'store') return 'source_store'
  return null
}

export function aggregatePathDiffs(comparisons: DiffComparison[]): PathDiff[] {
  const map = new Map<string, PathDiff>()
  const get = (path: string): PathDiff => {
    let row = map.get(path)
    if (!row) {
      row = { path, upstream: 'unchanged', store: 'unchanged', entries: [] }
      map.set(path, row)
    }
    return row
  }

  for (const comparison of comparisons) {
    const side = comparisonSide(comparison.from, comparison.to)
    if (!side) continue
    for (const entry of comparison.entries) {
      const row = get(entry.path)
      const state = changesToState(entry.changes)
      if (side === 'upstream') row.upstream = state
      else if (side === 'store') row.store = state
      row.entries.push({ side, entry })
    }
  }

  return [...map.values()].sort((a, b) => a.path.localeCompare(b.path))
}

export function comparisonsAreEmpty(comparisons: DiffComparison[]): boolean {
  return comparisons.every((comparison) => comparison.entries.length === 0)
}

export function expandedDiffEntries(row: PathDiff): { side: DiffSide; entry: DiffEntry }[] {
  // Source→Store restates a Baseline comparison as the opposite add/delete.
  const primary = row.entries.filter((item) => item.side !== 'source_store')
  return primary.length > 0 ? primary : row.entries
}
