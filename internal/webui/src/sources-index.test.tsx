import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourcesIndex, SourceExplorer } from './sources'
import { setupMatchMedia, summary, detail, mockResponse } from './source-fixtures'

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

setupMatchMedia()

function renderIndex() {
  return render(
    <MemoryRouter initialEntries={['/sources']}>
      <Routes>
        <Route path="/sources" element={<SourcesIndex />} />
        <Route path="/sources/:name" element={<SourceExplorer />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('SourcesIndex', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (url === '/api/v1/sources') {
        return mockResponse({ items: [summary], total: 1 })
      }
      return mockResponse({ error: { message: 'unexpected call' } }, false)
    })
  })

  afterEach(() => cleanup())

  it('renders registered sources with status badges', async () => {
    renderIndex()
    expect(await screen.findByRole('link', { name: 'local-one' })).toBeTruthy()
    expect(screen.getByText('/tmp/skills')).toBeTruthy()
    expect(screen.getByText('Available')).toBeTruthy()
    expect(screen.getAllByText('local').length).toBeGreaterThan(0)
    const nameCell = screen.getByRole('link', { name: 'local-one' }).closest('td')
    expect(nameCell?.querySelector('svg[data-source-provider="local"]')).toBeTruthy()
  })

  it('submits the add form and opens the created Source detail', async () => {
    const user = userEvent.setup()
    renderIndex()
    await screen.findByRole('link', { name: 'local-one' })

    const created = { ...detail, id: 7, name: 'new-skills', location: '/tmp/new-skills' }
    const post = vi.fn().mockResolvedValue(mockResponse(created, true, 201))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return post(url, init)
      if (String(url) === '/api/v1/sources/7') return mockResponse(created)
      return mockResponse({ items: [summary, created], total: 2 })
    })

    expect(screen.queryByText('Subpath (optional)')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Add Source' }))
    const location = screen.getByPlaceholderText('/absolute/path/to/skills')
    await user.type(location, '/tmp/new-skills')
    await user.click(screen.getByRole('button', { name: /Register and scan/ }))

    await waitFor(() => expect(post).toHaveBeenCalled())
    const [url, init] = post.mock.calls[0]
    expect(url).toBe('/api/v1/sources')
    expect(JSON.parse(init.body)).toEqual({ kind: 'local', location: '/tmp/new-skills' })
    // the created Source detail opens instead of only refreshing the list
    expect(await screen.findByRole('heading', { name: 'new-skills' })).toBeTruthy()
    expect((await screen.findAllByText('/tmp/new-skills')).length).toBeGreaterThan(0)
  })

  it('keeps Git ref and subpath collapsed under Advanced options', async () => {
    const user = userEvent.setup()
    renderIndex()
    await screen.findByRole('link', { name: 'local-one' })

    expect(screen.queryByText('Subpath (optional)')).toBeNull()
    expect(screen.queryByText('Ref (optional)')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Advanced options' })).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Add Source' }))
    const kindSelect = screen.getByRole('combobox', { name: 'Kind' })
    expect(kindSelect.textContent).toContain('Local directory')
    await user.click(kindSelect)
    await user.click(await screen.findByRole('option', { name: 'Git repository' }))
    expect(kindSelect.textContent).toContain('Git repository')
    expect(kindSelect.textContent).not.toMatch(/^\s*git\s*$/)

    expect(screen.getByRole('button', { name: 'Advanced options' })).toBeTruthy()
    expect(screen.queryByText('Subpath (optional)')).toBeNull()
    expect(screen.queryByText('Ref (optional)')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Advanced options' }))

    expect(screen.getByText('Subpath (optional)')).toBeTruthy()
    expect(screen.getByText('Ref (optional)')).toBeTruthy()
  })

  it('shows a spinner while loading and replaces it with content', async () => {
    let resolve!: (r: Response) => void
    vi.mocked(window.fetch).mockReturnValue(new Promise((res) => { resolve = res }))
    renderIndex()
    expect(screen.getByLabelText('Loading sources')).toBeTruthy()
    resolve(mockResponse({ items: [summary], total: 1 }))
    expect(await screen.findByRole('link', { name: 'local-one' })).toBeTruthy()
    expect(screen.queryByLabelText('Loading sources')).toBeNull()
  })

  it('shows a load error instead of an endless spinner', async () => {
    vi.mocked(window.fetch).mockResolvedValue(mockResponse({ error: { message: 'server is down' } }, false))
    renderIndex()
    expect(await screen.findByText('server is down')).toBeTruthy()
    expect(screen.queryByLabelText('Loading sources')).toBeNull()
  })

  it('shows a registration error from the server', async () => {
    const user = userEvent.setup()
    renderIndex()
    await screen.findByRole('link', { name: 'local-one' })

    vi.mocked(window.fetch).mockImplementation(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return mockResponse({ error: { message: 'Source /tmp/x is not reachable' } }, false)
      }
      return mockResponse({ items: [summary], total: 1 })
    })

    await user.click(screen.getByRole('button', { name: 'Add Source' }))
    await user.type(screen.getByPlaceholderText('/absolute/path/to/skills'), '/tmp/x')
    await user.click(screen.getByRole('button', { name: /Register and scan/ }))

    expect(await screen.findByText('Source /tmp/x is not reachable')).toBeTruthy()
  })

  it('aborts registration and prevents navigation if unmounted', async () => {
    const user = userEvent.setup()
    renderIndex()
    await screen.findByRole('link', { name: 'local-one' })

    let postResolve!: (res: Response) => void
    const post = vi.fn().mockImplementation(() => new Promise((resolve) => { postResolve = resolve }))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') return post(url, init)
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })

    await user.click(screen.getByRole('button', { name: 'Add Source' }))
    const location = screen.getByPlaceholderText('/absolute/path/to/skills')
    await user.type(location, '/tmp/new')
    await user.click(screen.getByRole('button', { name: /Register and scan/ }))

    await waitFor(() => expect(post).toHaveBeenCalled())
    const init = post.mock.calls[0][1] as RequestInit

    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(init.signal?.aborted).toBe(true)

    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const created = { ...detail, id: 8, name: 'new', location: '/tmp/new' }
    postResolve(mockResponse(created, true, 201))
    await new Promise((r) => setTimeout(r, 10)) // wait for microtasks

    expect(screen.queryByRole('heading', { name: 'new' })).toBeNull()
    expect(screen.getByRole('heading', { name: 'Sources' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'local-one' })).toBeTruthy()

    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('renders a horizontal ScrollArea for the sources table', async () => {
    renderIndex()
    await screen.findByRole('link', { name: 'local-one' })
    const scrollArea = screen.getByTestId('mock-scroll-area')
    expect(scrollArea.getAttribute('data-orientation')).toBe('horizontal')
  })
})
