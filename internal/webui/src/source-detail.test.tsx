import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ToastProvider, Toaster } from '@appica/ui-react/toast'
import { SourceDetailPage } from './source-detail'
import { NotifySuccessBridge } from './notify-success'
import { SourcesIndex } from './sources'
import { setupMatchMedia, summary, detail, unavailableDetail, mockResponse } from './source-fixtures'
import { skillAlpha } from './skill-fixtures'
import type { SourceDetail } from './source-api'

setupMatchMedia()

vi.mock('@appica/ui-react/scroll-area', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@appica/ui-react/scroll-area')>()
  return {
    ...actual,
    ScrollArea: (props: any) => (
      <div data-testid="mock-scroll-area" data-orientation={props.orientation}>
        {actual.ScrollArea ? actual.ScrollArea(props) : props.children}
      </div>
    )
  }
})

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={['/sources/local-one']}>
      <ToastProvider>
        <NotifySuccessBridge>
          <Routes>
            <Route path="/sources" element={<SourcesIndex />} />
            <Route path="/sources/:name" element={<SourceDetailPage />} />
          </Routes>
        </NotifySuccessBridge>
        <Toaster />
      </ToastProvider>
    </MemoryRouter>,
  )
}

function mockDetailResponse(overrides: Partial<SourceDetail>) {
  const custom = { ...detail, ...overrides }
  vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
    if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
    return mockResponse(custom)
  })
}

describe('SourceDetailPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })
  })

  afterEach(() => cleanup())

  it('renders the Inventory table with the relative directory inside the skill name cell', async () => {
    renderDetail()
    expect(await screen.findByRole('heading', { name: 'local-one' })).toBeTruthy()
    expect(screen.getByText('Alpha')).toBeTruthy()
    expect(screen.getByText('Beta')).toBeTruthy()
    expect(screen.getByText(/Inventory \(2\)/)).toBeTruthy()
    expect(screen.getByText('/tmp/skills')).toBeTruthy()

    // no standalone Directory column; relative_dir sits under the name
    expect(screen.queryByRole('columnheader', { name: 'Directory' })).toBeNull()
    const nameCell = screen.getByText('Alpha').closest('td') as HTMLElement
    expect(within(nameCell).getByText('skills/alpha')).toBeTruthy()
  })

  it('rescans through the API and refreshes the Inventory', async () => {
    const user = userEvent.setup()
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const refreshed = { ...detail, inventory: [...detail.inventory, { relative_dir: 'skills/gamma', name: 'Gamma', description: 'new' }] }
    const post = vi.fn().mockResolvedValue(mockResponse(refreshed))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return post(url, init)
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })

    await user.click(screen.getByRole('button', { name: /Rescan/ }))

    await waitFor(() => expect(post).toHaveBeenCalledWith('/api/v1/sources/1/check', expect.anything()))
    expect(await screen.findByText('Gamma')).toBeTruthy()
    expect(screen.getByText(/Inventory \(3\)/)).toBeTruthy()
    expect(await screen.findByText('Inventory updated: 1 added, 0 removed, 0 changed')).toBeTruthy()
  })

  it('renders the unavailable state with the retained Inventory', async () => {
    mockDetailResponse(unavailableDetail)
    renderDetail()
    expect(await screen.findByText('Unavailable')).toBeTruthy()
    expect(screen.getByText('Stale')).toBeTruthy()
    expect(screen.getByText(/no such directory/)).toBeTruthy()
    // the retained Inventory is still listed
    expect(screen.getByText('Alpha')).toBeTruthy()
  })

  it('places status beside the title and keeps kind inside details', async () => {
    const user = userEvent.setup()
    renderDetail()
    const heading = await screen.findByRole('heading', { name: 'local-one' })
    expect(within(heading.parentElement as HTMLElement).getByText('Available')).toBeTruthy()

    expect(screen.queryByText(/Kind:/)).toBeNull()
    expect(screen.queryByText(/Last scanned:.*\d{4}/)).toBeNull()
    expect(screen.queryByText(/Ref:/)).toBeNull()
    expect(screen.queryByText(/Subpath:/)).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Details' }))

    expect(screen.getByText(/Kind:/)).toBeTruthy()
    expect(screen.getByText('local')).toBeTruthy()
    expect(screen.getByText(/Ref:/)).toBeTruthy()
    expect(screen.queryByText(/Subpath:/)).toBeNull()
    expect(screen.getAllByText(/Last scanned:/).length).toBeGreaterThan(0)
  })

  function mockSkillsResponse() {
    const bound = {
      ...skillAlpha,
      binding: { ...skillAlpha.binding!, source_id: 1, relative_dir: 'skills/alpha' },
    }
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [bound], total: 1 })
      return mockResponse(detail)
    })
  }

  it('gathers the Source actions into one toolbar, Synchronize beside Rescan', async () => {
    mockSkillsResponse()
    renderDetail()

    const rescan = await screen.findByRole('button', { name: 'Rescan' })
    const sync = screen.getByRole('button', { name: 'Synchronize now' })
    const importAll = screen.getByRole('button', { name: 'Import all' })
    expect(rescan.compareDocumentPosition(sync) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(sync.compareDocumentPosition(importAll) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    // What each button does, and how old its reading is, lives on hover now.
    expect(screen.queryByText(/A rescan only refreshes this list/)).toBeNull()
    expect(screen.queryByText(/Last scanned/)).toBeNull()
    expect(screen.queryByText(/Sync Status last evaluated/)).toBeNull()
  })

  it('offers no Synchronize and says why on an untouched Source', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })
    expect(screen.getByText(/Import from this Inventory to synchronize upstream changes/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Synchronize now' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Import all' })).toBeTruthy()
  })

  it('presents the resolved Git commit only after expanding details', async () => {
    const user = userEvent.setup()
    mockDetailResponse({
      kind: 'git',
      location: 'https://git.example.com/org/repo',
      last_commit: 'a'.repeat(40),
    })
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    expect(screen.queryByText(/Resolved commit:/)).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Details' }))

    expect(screen.getByText(/Subpath:/)).toBeTruthy()
    expect(screen.getByText(/Resolved commit:/)).toBeTruthy()
    expect(screen.getByText('a'.repeat(40))).toBeTruthy()
  })

  it('renders https locations as safe external links and other location forms as plain text', async () => {
    const cases: Array<{ location: string; asLink: boolean }> = [
      { location: '/tmp/skills', asLink: false },
      { location: 'https://github.com/org/repo', asLink: true },
      { location: 'https://gitlab.com/group/project', asLink: true },
      { location: 'git@git.example.com:org/repo.git', asLink: false },
    ]
    for (const { location, asLink } of cases) {
      cleanup()
      mockDetailResponse(location.startsWith('/') ? { location } : { kind: 'git', location })
      renderDetail()
      await screen.findByRole('heading', { name: 'local-one' })

      if (asLink) {
        const link = screen.getByRole('link', { name: location })
        expect(link.getAttribute('href')).toBe(location)
        expect(link.getAttribute('target')).toBe('_blank')
        expect(link.getAttribute('rel')).toContain('noopener')
        expect(link.getAttribute('rel')).toContain('noreferrer')
      } else {
        expect(screen.getByText(location).closest('a')).toBeNull()
      }
    }
  })

  it('keeps the checkbox column sticky with opaque cells for horizontal scrolling', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const headerCell = screen.getByRole('checkbox', { name: 'Select all on this page (2)' }).closest('th')
    expect(headerCell?.className).toContain('sticky')
    expect(headerCell?.className).toContain('left-0')
    expect(headerCell?.className).toContain('bg-background-muted')

    const bodyCell = screen.getByRole('checkbox', { name: 'Select Alpha' }).closest('td')
    expect(bodyCell?.className).toContain('sticky')
    expect(bodyCell?.className).toContain('left-0')
    expect(bodyCell?.className).toContain('bg-background')
  })

  it('shows a load error instead of an endless spinner on the detail page', async () => {
    vi.mocked(window.fetch).mockResolvedValue(mockResponse({ error: { message: 'state database is missing' } }, false))
    renderDetail()
    expect(await screen.findByText('state database is missing')).toBeTruthy()
    expect(screen.queryByLabelText('Loading source')).toBeNull()
  })

  it('renders invalid entries as warnings', async () => {
    mockDetailResponse({
      issues: [{ relative_dir: 'skills/bad', reason: 'frontmatter name is required' }],
    })
    renderDetail()
    expect(await screen.findByText('1 invalid entry skipped')).toBeTruthy()
    expect(screen.getByText(/frontmatter name is required/)).toBeTruthy()
  })

  it('aborts the check signal and ignores completion if unmounted', async () => {
    const user = userEvent.setup()
    const { unmount } = renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    let checkResolve!: (res: Response) => void
    const post = vi.fn().mockImplementation(() => new Promise((resolve) => { checkResolve = resolve }))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return post(url, init)
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })

    await user.click(screen.getByRole('button', { name: /Rescan/ }))
    await waitFor(() => expect(post).toHaveBeenCalled())
    const init = post.mock.calls[0][1] as RequestInit

    unmount()
    expect(init.signal?.aborted).toBe(true)

    // Resolve after unmount; should not throw or attempt state updates
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    checkResolve(mockResponse({ ...detail, name: 'should-be-ignored' }))
    await new Promise((r) => setTimeout(r, 10)) // wait for microtasks

    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('renders a horizontal ScrollArea for the Inventory table', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })
    const scrollArea = screen.getByTestId('mock-scroll-area')
    expect(scrollArea.getAttribute('data-orientation')).toBe('horizontal')
  })
})
