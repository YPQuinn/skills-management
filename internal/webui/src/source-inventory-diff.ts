import type { SourceEntry } from './source-api'

export interface InventoryChange {
  added: number
  removed: number
  changed: number
}

// diffInventory compares two Inventory samples by relative directory, the
// identity the Source Binding uses. An entry counts as changed only when
// both samples carry a digest and the digests differ, so a Source that does
// not report digests is never described as changed.
export function diffInventory(before: SourceEntry[], after: SourceEntry[]): InventoryChange {
  const previous = new Map(before.map((e) => [e.relative_dir, e]))
  let added = 0
  let changed = 0
  for (const entry of after) {
    const prior = previous.get(entry.relative_dir)
    if (!prior) {
      added++
      continue
    }
    if (prior.digest && entry.digest && prior.digest !== entry.digest) changed++
    previous.delete(entry.relative_dir)
  }
  return { added, removed: previous.size, changed }
}

export function inventoryUnchanged(change: InventoryChange): boolean {
  return change.added === 0 && change.removed === 0 && change.changed === 0
}
