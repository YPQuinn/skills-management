import { describe, it, expect } from 'vitest'
import { aggregatePathDiffs, comparisonsAreEmpty, expandedDiffEntries } from './sync-diff-paths'
import { diffResult } from './skill-sync-fixtures'

describe('aggregatePathDiffs', () => {
  it('collapses the three comparisons into one row per path with result-language sides', () => {
    const rows = aggregatePathDiffs(diffResult.comparisons)
    expect(rows.map((row) => row.path)).toEqual(['kind-change', 'logo.png', 'notes.md', 'old.md', 'run.sh'])

    const notes = rows.find((row) => row.path === 'notes.md')
    expect(notes?.upstream).toBe('added')
    expect(notes?.store).toBe('unchanged')

    const kind = rows.find((row) => row.path === 'kind-change')
    expect(kind?.upstream).toBe('unchanged')
    expect(kind?.store).toBe('unchanged')
  })

  it('does not invent Upstream/Store chip states from a Source vs Store-only path', () => {
    const rows = aggregatePathDiffs([
      { from: 'baseline', to: 'source', entries: [] },
      { from: 'baseline', to: 'store', entries: [] },
      {
        from: 'source',
        to: 'store',
        entries: [{ path: 'only-cross.md', changes: ['add'] }],
      },
    ])
    expect(rows).toEqual([
      expect.objectContaining({ path: 'only-cross.md', upstream: 'unchanged', store: 'unchanged' }),
    ])
  })

  it('hides Source vs Store delete when Upstream already accounts for the path', () => {
    const rows = aggregatePathDiffs([
      { from: 'baseline', to: 'source', entries: [{ path: 'notes.md', changes: ['add'] }] },
      { from: 'baseline', to: 'store', entries: [] },
      { from: 'source', to: 'store', entries: [{ path: 'notes.md', changes: ['delete'] }] },
    ])
    const notes = rows.find((row) => row.path === 'notes.md')
    expect(notes?.upstream).toBe('added')
    expect(notes?.store).toBe('unchanged')
    const panels = expandedDiffEntries(notes!)
    expect(panels.map((item) => item.side)).toEqual(['upstream'])
    expect(panels.some((item) => item.entry.changes.includes('delete'))).toBe(false)
  })

  it('keeps Source vs Store when it is the only comparison for the path', () => {
    const rows = aggregatePathDiffs([
      { from: 'baseline', to: 'source', entries: [] },
      { from: 'baseline', to: 'store', entries: [] },
      {
        from: 'source',
        to: 'store',
        entries: [{ path: 'kind-change', changes: ['node_type'] }],
      },
    ])
    const kind = rows.find((row) => row.path === 'kind-change')
    expect(expandedDiffEntries(kind!).map((item) => item.side)).toEqual(['source_store'])
    expect(expandedDiffEntries(kind!)[0].entry.changes).toEqual(['node_type'])
  })

  it('treats fully empty comparisons as a single empty state', () => {
    expect(comparisonsAreEmpty(diffResult.comparisons)).toBe(false)
    expect(comparisonsAreEmpty(diffResult.comparisons.map((c) => ({ ...c, entries: [] })))).toBe(true)
  })
})
