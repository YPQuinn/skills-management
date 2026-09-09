import { describe, it, expect } from 'vitest'
import { diffInventory, inventoryUnchanged } from './source-inventory-diff'
import type { SourceEntry } from './source-api'

function entry(dir: string, digest?: string): SourceEntry {
  return { relative_dir: dir, name: dir, description: '', ...(digest ? { digest } : {}) }
}

describe('diffInventory', () => {
  it('reports an identical sample as unchanged', () => {
    const sample = [entry('skills/alpha', 'sha256:a'), entry('skills/beta', 'sha256:b')]
    const change = diffInventory(sample, [...sample])
    expect(change).toEqual({ added: 0, removed: 0, changed: 0 })
    expect(inventoryUnchanged(change)).toBe(true)
  })

  it('counts added, removed, and digest-changed entries by relative directory', () => {
    const change = diffInventory(
      [entry('skills/alpha', 'sha256:a'), entry('skills/beta', 'sha256:b'), entry('skills/gone', 'sha256:g')],
      [entry('skills/alpha', 'sha256:a2'), entry('skills/beta', 'sha256:b'), entry('skills/new', 'sha256:n')],
    )
    expect(change).toEqual({ added: 1, removed: 1, changed: 1 })
    expect(inventoryUnchanged(change)).toBe(false)
  })

  it('never reports a change when either sample omits the digest', () => {
    expect(diffInventory([entry('skills/alpha')], [entry('skills/alpha', 'sha256:a')])).toEqual({
      added: 0,
      removed: 0,
      changed: 0,
    })
    expect(diffInventory([entry('skills/alpha', 'sha256:a')], [entry('skills/alpha')])).toEqual({
      added: 0,
      removed: 0,
      changed: 0,
    })
  })

  it('treats a first observation and an emptied Source as pure add and remove', () => {
    expect(diffInventory([], [entry('skills/alpha'), entry('skills/beta')])).toEqual({
      added: 2,
      removed: 0,
      changed: 0,
    })
    expect(diffInventory([entry('skills/alpha')], [])).toEqual({ added: 0, removed: 1, changed: 0 })
  })
})
