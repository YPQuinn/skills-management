import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SourceSyncSection } from './source-sync'
import { mockResponse } from './source-fixtures'
import type { SyncBatchResult } from './sync-api'

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
        <SourceSyncSection sourceId={1} onSynced={onSynced} />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: 'Synchronize bound Skills' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Sync bound Skills' }))

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(([u]) => String(u) === '/api/v1/sources/1/sync')
      expect(post).toBeTruthy()
      expect(post![1]).toMatchObject({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
    })

    expect(await screen.findByText('Batch sync completed')).toBeTruthy()
    expect(screen.getByText(/Total: 2; 1 updated, 0 kept, 0 accepted, 0 no-op, 1 skipped/)).toBeTruthy()

    // Per-item rows: skill links, status and result badges, skipped message.
    const wayfinder = screen.getByRole('link', { name: 'wayfinder' })
    expect(wayfinder.getAttribute('href')).toBe('/skills/wayfinder?tab=synchronization')
    expect(screen.getByRole('link', { name: 'grilling' })).toBeTruthy()
    expect(screen.getByText('Updated')).toBeTruthy()
    expect(screen.getByText('Skipped')).toBeTruthy()
    expect(screen.getByText('Source and Store both changed since the Baseline')).toBeTruthy()
    expect(onSynced).toHaveBeenCalledTimes(1)
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
        <SourceSyncSection sourceId={1} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Sync bound Skills' }))

    expect(await screen.findByText('Source synchronization failed')).toBeTruthy()
    expect(screen.getByText('the Source is not reachable')).toBeTruthy()
  })

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
        <SourceSyncSection sourceId={1} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: 'Sync bound Skills' }))
    expect(await screen.findByText('No Skills are bound to this Source.')).toBeTruthy()
  })
})
