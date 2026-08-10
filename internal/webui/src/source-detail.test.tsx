import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourceDetailPage } from './source-detail'
import { SourcesIndex } from './sources'
import { setupMatchMedia, summary, detail, unavailableDetail, mockResponse } from './source-fixtures'

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

describe('SourceDetailPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })
  })

  afterEach(() => cleanup())

  it('renders the Inventory table and location facts', async () => {
    renderDetail()
    expect(await screen.findByRole('heading', { name: 'local-one' })).toBeTruthy()
    expect(screen.getByText('Alpha')).toBeTruthy()
    expect(screen.getByText('skills/alpha')).toBeTruthy()
    expect(screen.getByText('Beta')).toBeTruthy()
    expect(screen.getByText(/Inventory \(2\)/)).toBeTruthy()
    expect(screen.getByText('/tmp/skills')).toBeTruthy()
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
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(unavailableDetail)
    })
    renderDetail()
    expect(await screen.findByText('Unavailable')).toBeTruthy()
    expect(screen.getByText('Stale')).toBeTruthy()
    expect(screen.getByText(/no such directory/)).toBeTruthy()
    // the retained Inventory is still listed
    expect(screen.getByText('Alpha')).toBeTruthy()
  })

  it('presents the resolved Git commit', async () => {
    const gitDetail = {
      ...detail,
      kind: 'git',
      last_commit: 'a'.repeat(40),
    }
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(gitDetail)
    })
    renderDetail()
    expect(await screen.findByText('Resolved commit')).toBeTruthy()
    expect(screen.getByText('a'.repeat(40))).toBeTruthy()
  })

  it('shows a load error instead of an endless spinner on the detail page', async () => {
    vi.mocked(window.fetch).mockResolvedValue(mockResponse({ error: { message: 'state database is missing' } }, false))
    renderDetail()
    expect(await screen.findByText('state database is missing')).toBeTruthy()
    expect(screen.queryByLabelText('Loading source')).toBeNull()
  })

  it('renders invalid entries as warnings', async () => {
    const withIssues = {
      ...detail,
      issues: [{ relative_dir: 'skills/bad', reason: 'frontmatter name is required' }],
    }
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(withIssues)
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
