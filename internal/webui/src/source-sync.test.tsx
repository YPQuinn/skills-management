import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { TooltipProvider } from '@appica/ui-react/tooltip'
import { ToastProvider, Toaster } from '@appica/ui-react/toast'
import { NotifySuccessBridge } from './notify-success'
import { useSourceSync, SourceSyncButton, SourceSyncStatus } from './source-sync'
import { summarizeBoundSync } from './source-sync-summary'
import { mockResponse } from './source-fixtures'
import { skillAlpha } from './skill-fixtures'
import type { Skill } from './skill-api'
import type { SyncBatchResult } from './sync-api'

// Mirrors how the Source page composes the pieces: one hook driving a
// toolbar button and a status block that sit in different rows. The Toaster
// is real because a run now reports its outcomes through a toast.
function SyncPieces({ boundSkills, onSynced }: { boundSkills: Skill[]; onSynced?: () => void }) {
  const sync = useSourceSync(1, onSynced)
  return (
    <>
      {boundSkills.length > 0 && (
        <SourceSyncButton
          syncing={sync.syncing}
          onRun={sync.run}
          lastEvaluatedAt={summarizeBoundSync(boundSkills).lastEvaluatedAt}
        />
      )}
      <SourceSyncStatus boundSkills={boundSkills} error={sync.error} />
    </>
  )
}

function SourceSyncSection(props: { boundSkills: Skill[]; onSynced?: () => void }) {
  return (
    <TooltipProvider delay={0}>
      <ToastProvider>
        <NotifySuccessBridge>
          <SyncPieces {...props} />
        </NotifySuccessBridge>
        <Toaster />
      </ToastProvider>
    </TooltipProvider>
  )
}

const recently = new Date(Date.now() - 5 * 60_000).toISOString()

function boundSkill(slug: string, status: string, checkedAt: string = recently): Skill {
  return { ...skillAlpha, slug, sync_status: status, sync_checked_at: checkedAt }
}

function setupMatchMedia(): void {
  window.matchMedia =
    window.matchMedia ||
    function () {
      return {
        matches: false,
        addListener: function () {},
        removeListener: function () {},
        addEventListener: function () {},
        removeEventListener: function () {},
        dispatchEvent: function () {},
      }
    }
}
setupMatchMedia()

const batchResult: SyncBatchResult = {
  items: [
    {
      skill_id: 11,
      slug: 'wayfinder',
      status: 'in_sync',
      stale: false,
      action: 'sync',
      result: 'updated',
      before_digest: 'sha256:old',
      after_digest: 'sha256:new',
      revision: 'abc1234',
    },
    {
      skill_id: 12,
      slug: 'grilling',
      status: 'conflict',
      stale: false,
      action: 'sync',
      result: 'skipped',
      message: 'Source and Store both changed since the Baseline',
    },
  ],
  summary: {
    total: 2,
    no_op: 0,
    updated: 1,
    kept_store: 0,
    accepted_source: 0,
    skipped: 1,
    blocked: 0,
    failed: 0,
    rolled_back: 0,
  },
}

describe('Source Batch Synchronization', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  function installFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
    const mock = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init))
    window.fetch = mock as unknown as typeof fetch
    return mock
  }

  it('triggers the batch POST and renders per-item outcomes plus the summary', async () => {
    const onSynced = vi.fn()
    const fetchMock = installFetch((url) => {
      if (String(url) === '/api/v1/sources/1/sync') return mockResponse(batchResult)
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[boundSkill('wayfinder', 'in_sync'), boundSkill('grilling', 'source_changed')]}
          onSynced={onSynced}
        />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Synchronize now' }))

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(([u]) => String(u) === '/api/v1/sources/1/sync')
      expect(post).toBeTruthy()
      expect(post![1]).toMatchObject({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
    })

    // The tally and the outcomes now arrive as a toast, not as page content.
    expect(await screen.findByText('Synchronization finished')).toBeTruthy()
    expect(screen.getByText('1 Updated')).toBeTruthy()
    expect(screen.getByText('1 Skipped')).toBeTruthy()
    // Empty buckets are dropped rather than printed as a run of zeros.
    expect(screen.queryByText(/0 Blocked|0 No-op/)).toBeNull()

    const grilling = screen.getByRole('link', { name: 'grilling' })
    expect(grilling.getAttribute('href')).toBe('/skills/grilling?tab=synchronization')
    expect(screen.getByText('Source and Store both changed since the Baseline')).toBeTruthy()
    expect(onSynced).toHaveBeenCalledTimes(1)
  })

  it('names only the Skills a run left for the reader to deal with', async () => {
    installFetch((url) => {
      if (String(url) === '/api/v1/sources/1/sync') return mockResponse(batchResult)
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[boundSkill('wayfinder', 'in_sync'), boundSkill('grilling', 'conflict')]}
        />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Synchronize now' }))
    expect(await screen.findByRole('link', { name: 'grilling' })).toBeTruthy()

    // wayfinder updated cleanly, so the toast does not spend a row on it;
    // the tally already counted it.
    expect(screen.queryByRole('link', { name: 'wayfinder' })).toBeNull()

    // The page keeps the durable signal: gamma-style conflicts still raise
    // the standing Alert, which no longer steps aside for a results list.
    expect(screen.getByText(/Keep Store or Accept Source/)).toBeTruthy()
  })

  it('surfaces a failed batch with the server message', async () => {
    installFetch((url) => {
      if (String(url) === '/api/v1/sources/1/sync') {
        return mockResponse({ error: { message: 'the Source is not reachable' } }, false, 409)
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()

    render(
      <MemoryRouter>
        <SourceSyncSection boundSkills={[boundSkill('wayfinder', 'in_sync')]} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Synchronize now' }))

    expect(await screen.findByText('Source synchronization failed')).toBeTruthy()
    expect(screen.getByText('the Source is not reachable')).toBeTruthy()
  })

  it('offers no action and explains itself when no Skill is bound', () => {
    const fetchMock = installFetch(() => mockResponse({ error: { message: 'Not found' } }, false, 404))

    render(
      <MemoryRouter>
        <SourceSyncSection boundSkills={[]} />
      </MemoryRouter>,
    )

    expect(screen.queryByRole('button', { name: 'Synchronize now' })).toBeNull()
    expect(screen.getByText(/Import from this Inventory to synchronize upstream changes/)).toBeTruthy()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('badges only the statuses needing attention', () => {
    installFetch(() => mockResponse({ error: { message: 'Not found' } }, false, 404))

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[
            boundSkill('wayfinder', 'in_sync'),
            boundSkill('grilling', 'source_changed'),
            boundSkill('tdd', 'in_sync'),
          ]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('1 Source changed')).toBeTruthy()
    // Settled Skills are not news, so they earn no badge of their own.
    expect(screen.queryByText('2 In sync')).toBeNull()
    expect(screen.queryByText(/match their Source/)).toBeNull()
  })

  // The description and the reading's age moved onto the button's Tooltip.
  // Base UI opens it on real pointer events, which jsdom does not deliver, so
  // this pins the layout half of that move: neither costs a row any more.
  it('spends no layout on the description or the age of the reading', () => {
    installFetch(() => mockResponse({ error: { message: 'Not found' } }, false, 404))

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[boundSkill('wayfinder', 'in_sync'), boundSkill('tdd', 'in_sync')]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByRole('button', { name: 'Synchronize now' })).toBeTruthy()
    expect(screen.queryByText(/Sync Status last evaluated/)).toBeNull()
    expect(screen.queryByText(/Pulls upstream changes into the Skills bound/)).toBeNull()
  })

  it('states one conclusion instead of badges when every bound Skill matches', () => {
    installFetch(() => mockResponse({ error: { message: 'Not found' } }, false, 404))

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[boundSkill('wayfinder', 'in_sync'), boundSkill('tdd', 'in_sync')]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('All 2 bound Skills match their Source.')).toBeTruthy()
  })

  it('states a conflict once, in the Alert that names the two resolutions', () => {
    installFetch(() => mockResponse({ error: { message: 'Not found' } }, false, 404))

    render(
      <MemoryRouter>
        <SourceSyncSection
          boundSkills={[boundSkill('wayfinder', 'conflict'), boundSkill('grilling', 'in_sync')]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('Sync conflict')).toBeTruthy()
    expect(
      screen.getByText(
        "Synchronization will skip 1 of them until you Keep Store or Accept Source on the Skill's Synchronization tab.",
      ),
    ).toBeTruthy()
    // The Alert is the single signal: no duplicate red badge beside it.
    expect(screen.queryByText('1 Sync conflict')).toBeNull()
  })

  // A Source with dozens of Bindings can strand dozens of Skills at once,
  // and a toast tall enough to list them all would cover the page.
  it('caps the named Skills and points at the index for the rest', async () => {
    const many = Array.from({ length: 8 }, (_, i) => ({
      skill_id: 100 + i,
      slug: `stuck-${i}`,
      status: 'conflict',
      stale: false,
      action: 'sync',
      result: 'skipped',
    }))
    installFetch((url) => {
      if (String(url) === '/api/v1/sources/1/sync') {
        return mockResponse({
          items: many,
          summary: { total: 8, no_op: 0, updated: 0, kept_store: 0, accepted_source: 0, skipped: 8, blocked: 0, failed: 0, rolled_back: 0 },
        })
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()

    render(
      <MemoryRouter>
        <SourceSyncSection boundSkills={[boundSkill('wayfinder', 'conflict')]} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Synchronize now' }))

    expect(await screen.findByRole('link', { name: 'stuck-4' })).toBeTruthy()
    expect(screen.queryByRole('link', { name: 'stuck-5' })).toBeNull()
    expect(screen.getByText(/…and 3 more needing attention/)).toBeTruthy()
  })

  // A Binding can vanish between render and click, so the batch can still
  // come back empty even though the button was enabled.
  it('shows the empty-bound set message when the batch has no items', async () => {
    installFetch((url) => {
      if (String(url) === '/api/v1/sources/1/sync') {
        return mockResponse({
          items: [],
          summary: { total: 0, no_op: 0, updated: 0, kept_store: 0, accepted_source: 0, skipped: 0, blocked: 0, failed: 0, rolled_back: 0 },
        })
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()

    render(
      <MemoryRouter>
        <SourceSyncSection boundSkills={[boundSkill('wayfinder', 'in_sync')]} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Synchronize now' }))
    expect(await screen.findByText('No Skills are bound to this Source.')).toBeTruthy()
  })
})
