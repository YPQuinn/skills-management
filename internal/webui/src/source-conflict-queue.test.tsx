import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourceDetailPage } from './source-detail'
import { setupMatchMedia, summary, detail, mockResponse } from './source-fixtures'

setupMatchMedia()

vi.mock('@appica/ui-react/scroll-area', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@appica/ui-react/scroll-area')>()
  return {
    ...actual,
    ScrollArea: (props: any) => (
      <div data-testid="mock-scroll-area" data-orientation={props.orientation}>
        {actual.ScrollArea ? actual.ScrollArea(props) : props.children}
      </div>
    ),
  }
})

const mockMultiConflictResponse = {
  items: [
    {
      status: 'skipped_conflict',
      relative_dir: 'skills/alpha',
      replaces: { skill_id: 101, slug: 'alpha', name: 'Alpha Skill' },
    },
    {
      status: 'skipped_conflict',
      relative_dir: 'skills/beta',
      replaces: { skill_id: 102, slug: 'beta', name: 'Beta Skill' },
    },
  ],
  summary: { total: 2, imported: 0, already_imported: 0, skipped_conflict: 2, replaced: 0, failed: 0 },
}

const mockReplaceAlphaResult = {
  items: [{ status: 'replaced', relative_dir: 'skills/alpha', slug: 'alpha', skill_id: 101 }],
  summary: { total: 1, imported: 0, already_imported: 0, skipped_conflict: 0, replaced: 1, failed: 0 },
}

const mockReplaceBetaResult = {
  items: [{ status: 'replaced', relative_dir: 'skills/beta', slug: 'beta', skill_id: 102 }],
  summary: { total: 1, imported: 0, already_imported: 0, skipped_conflict: 0, replaced: 1, failed: 0 },
}

function renderSourceDetail() {
  return render(
    <MemoryRouter initialEntries={['/sources/local-one']}>
      <Routes>
        <Route path="/sources/:name" element={<SourceDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Source Import Multi-Conflict FIFO Queue', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('processes multi-conflict queue: replace-first and decline-second', async () => {
    const user = userEvent.setup()
    const importPost = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(mockMultiConflictResponse))
      .mockResolvedValueOnce(mockResponse(mockReplaceAlphaResult))

    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        return importPost(url, init)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    await user.click(screen.getByRole('button', { name: 'Import all' }))

    // First conflict in queue: Alpha Skill
    expect(await screen.findByText('Replace existing Skill?')).toBeTruthy()
    expect(screen.getByText(/Alpha Skill/)).toBeTruthy()

    // Replace Alpha Skill
    await user.click(screen.getByRole('button', { name: 'Replace Skill' }))

    // Queue advances to second conflict: Beta Skill
    expect(await screen.findByText(/Beta Skill/)).toBeTruthy()

    // Decline (Cancel) Beta Skill
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    // Dialog closes when queue is drained
    await waitFor(() => {
      expect(screen.queryByText('Replace existing Skill?')).toBeNull()
    })

    // Summary recomputed: 1 replaced, 1 conflict
    expect(
      screen.getByText(/Total: 2, 0 imported, 1 replaced, 0 already imported, 1 conflict, 0 failed/),
    ).toBeTruthy()
  })

  it('processes multi-conflict queue: decline-first and replace-second', async () => {
    const user = userEvent.setup()
    const importPost = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(mockMultiConflictResponse))
      .mockResolvedValueOnce(mockResponse(mockReplaceBetaResult))

    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        return importPost(url, init)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    await user.click(screen.getByRole('button', { name: 'Import all' }))

    // First conflict: Alpha Skill
    expect(await screen.findByText(/Alpha Skill/)).toBeTruthy()

    // Decline Alpha Skill
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    // Queue advances to Beta Skill
    expect(await screen.findByText(/Beta Skill/)).toBeTruthy()

    // Replace Beta Skill
    await user.click(screen.getByRole('button', { name: 'Replace Skill' }))

    // Queue drained and dialog closes
    await waitFor(() => {
      expect(screen.queryByText('Replace existing Skill?')).toBeNull()
    })

    // Summary recomputed: 1 replaced, 1 conflict
    expect(
      screen.getByText(/Total: 2, 0 imported, 1 replaced, 0 already imported, 1 conflict, 0 failed/),
    ).toBeTruthy()
  })

  it('failure keeps current queue head and retry advances upon success', async () => {
    const user = userEvent.setup()
    const importPost = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(mockMultiConflictResponse))
      .mockResolvedValueOnce(mockResponse({ error: { message: 'Conflict lock timeout' } }, false, 500))
      .mockResolvedValueOnce(mockResponse(mockReplaceAlphaResult))

    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        return importPost(url, init)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    await user.click(screen.getByRole('button', { name: 'Import all' }))

    expect(await screen.findByText(/Alpha Skill/)).toBeTruthy()

    // First replace attempt fails
    await user.click(screen.getByRole('button', { name: 'Replace Skill' }))

    // Failure message surfaces and Alpha Skill stays at queue head
    expect(await screen.findByText('Replace failed')).toBeTruthy()
    expect(screen.getByText('Conflict lock timeout')).toBeTruthy()
    expect(screen.getByText(/Alpha Skill/)).toBeTruthy()

    // Retry replace succeeds and advances to Beta Skill
    await user.click(screen.getByRole('button', { name: 'Replace Skill' }))

    expect(await screen.findByText(/Beta Skill/)).toBeTruthy()
  })

  it('renders preserved groups and targets with both direct and group reasons in replace impact preview', async () => {
    const user = userEvent.setup()
    const conflictWithImpact = {
      items: [
        {
          status: 'skipped_conflict',
          relative_dir: 'skills/alpha',
          replaces: { skill_id: 101, slug: 'alpha', name: 'Alpha Skill' },
          impact: {
            groups: [{ id: 1, name: 'backend-tools' }],
            targets: [
              {
                id: 10,
                name: 'Dev Server',
                direct: true,
                groups: [{ id: 1, name: 'backend-tools' }],
              },
            ],
          },
        },
      ],
      summary: { total: 1, imported: 0, already_imported: 0, skipped_conflict: 1, replaced: 0, failed: 0 },
    }

    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        return mockResponse(conflictWithImpact)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })
    await user.click(screen.getByRole('button', { name: 'Import all' }))

    expect(await screen.findByText('Preserved Memberships & Assignments')).toBeTruthy()
    expect(screen.getByText('Dev Server')).toBeTruthy()
    expect(screen.getByText('Direct')).toBeTruthy()
    expect(screen.getByText('via Group "backend-tools"')).toBeTruthy()
  })
})
