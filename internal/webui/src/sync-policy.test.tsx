import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SyncStatusBadge } from './sync-status'
import { syncActionsFor } from './sync-action-policy'
import type { SyncStatus } from './sync-api'
import { skillAlpha } from './skill-fixtures'
import { conflictSkill, setupMatchMedia } from './skill-sync-fixtures'

setupMatchMedia()

describe('Sync status badge policy', () => {
  it('maps every one of the ten sync states to a text badge', () => {
    const states: Array<[SyncStatus, string]> = [
      ['unbound', 'Unbound'],
      ['unchecked', 'Unchecked'],
      ['in_sync', 'In sync'],
      ['source_changed', 'Source changed'],
      ['store_changed', 'Store changed'],
      ['conflict', 'Sync conflict'],
      ['source_missing', 'Source entry missing'],
      ['source_invalid', 'Source entry invalid'],
      ['store_missing', 'Store content missing'],
      ['store_invalid', 'Store content invalid'],
    ]
    const { rerender } = render(
      <div>
        {states.map(([status]) => (
          <SyncStatusBadge key={status} status={status} />
        ))}
      </div>,
    )
    for (const [, label] of states) {
      expect(screen.getByText(label)).toBeTruthy()
    }
    rerender(<SyncStatusBadge status="mystery" />)
    expect(screen.getByText('mystery')).toBeTruthy()
  })
})

describe('Sync action policy', () => {
  it('offers only the legal actions for the current status', () => {
    expect(syncActionsFor(conflictSkill)).toEqual(['check', 'keep_store', 'accept_source', 'rollback'])
    expect(syncActionsFor({ ...conflictSkill, sync_status: 'in_sync' })).toEqual(['check', 'sync', 'rollback'])
    expect(syncActionsFor({ ...conflictSkill, sync_status: 'source_missing' })).toEqual(['check', 'sync', 'rollback'])
    expect(syncActionsFor(skillAlpha)).not.toContain('accept_source')
    expect(syncActionsFor({ ...conflictSkill, binding: undefined, sync_status: 'unbound' })).toEqual(['check'])
  })

  it('never offers rollback without a previous snapshot', () => {
    const noSnapshot = { ...conflictSkill, has_previous_snapshot: false }
    expect(syncActionsFor(noSnapshot)).not.toContain('rollback')
    expect(syncActionsFor(noSnapshot)).toEqual(['check', 'keep_store', 'accept_source'])
    // A Skill without a snapshot still offers the non-rollback actions.
    expect(syncActionsFor({ ...noSnapshot, sync_status: 'in_sync' })).toEqual(['check', 'sync'])
  })
})
