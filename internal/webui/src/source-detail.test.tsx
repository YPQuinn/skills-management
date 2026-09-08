import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourceDetailPage } from './source-detail'
import { SourcesIndex } from './sources'
import { setupMatchMedia, summary, detail, unavailableDetail, mockResponse } from './source-fixtures'
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
      <Routes>
        <Route path="/sources" element={<SourcesIndex />} />
        <Route path="/sources/:name" element={<SourceDetailPage />} />
      </Routes>
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

  it('re-checks through the API and refreshes the page', async () => {
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

    await user.click(screen.getByRole('button', { name: /Check again/ }))

    await waitFor(() => expect(post).toHaveBeenCalledWith('/api/v1/sources/1/check', expect.anything()))
    expect(await screen.findByText('Gamma')).toBeTruthy()
    expect(screen.getByText(/Inventory \(3\)/)).toBeTruthy()
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

  it('shows kind and last checked by default and expands the full facts on request', async () => {
    const user = userEvent.setup()
    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    // default subtle summary line near the title
    expect(screen.getByText(/Kind: local/)).toBeTruthy()
    expect(screen.getByText(/Last checked:/)).toBeTruthy()
    // detailed facts stay hidden until expanded
    expect(screen.queryByText(/Ref:/)).toBeNull()
    expect(screen.queryByText(/Subpath:/)).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Details' }))

    expect(screen.getByText(/Ref:/)).toBeTruthy()
    expect(screen.queryByText(/Subpath:/)).toBeNull()
    expect(screen.getAllByText(/Kind: local/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Last checked:/).length).toBeGreaterThan(0)
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

    await user.click(screen.getByRole('button', { name: /Check again/ }))
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
