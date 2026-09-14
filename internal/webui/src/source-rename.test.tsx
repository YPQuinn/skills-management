import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ToastProvider, Toaster } from '@appica/ui-react/toast'
import { SourceDetailPage } from './source-detail'
import { NotifySuccessBridge } from './notify-success'
import { SourcesIndex } from './sources'
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

const renamed = { ...detail, name: 'local-two' }
const renamedSummary = { ...summary, name: 'local-two' }

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

describe('Source rename', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse(detail)
    })
  })

  afterEach(() => cleanup())

  it('posts the new name and navigates to the renamed Source', async () => {
    const user = userEvent.setup()
    const post = vi.fn().mockResolvedValue(mockResponse(renamed))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url).endsWith('/rename')) return post(url, init)
      if (String(url) === '/api/v1/sources') {
        return mockResponse({ items: post.mock.calls.length ? [renamedSummary] : [summary], total: 1 })
      }
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse(post.mock.calls.length ? renamed : detail)
    })

    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    await user.click(screen.getByRole('button', { name: 'Rename' }))
    const dialog = await screen.findByRole('dialog')
    const input = within(dialog).getByRole('textbox')
    await user.clear(input)
    await user.type(input, 'local-two')
    await user.click(within(dialog).getByRole('button', { name: 'Rename' }))

    await waitFor(() => expect(post).toHaveBeenCalled())
    expect(post).toHaveBeenCalledWith('/api/v1/sources/1/rename', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ name: 'local-two' }),
    }))
    expect(await screen.findByRole('heading', { name: 'local-two' })).toBeTruthy()
    expect(screen.queryByRole('dialog', { name: 'Rename Source' })).toBeNull()
    expect(await screen.findByText('Source renamed')).toBeTruthy()
  })

  it('keeps the dialog open and shows the conflict on failure', async () => {
    const user = userEvent.setup()
    const post = vi.fn().mockResolvedValue(mockResponse(
      { error: { message: 'a Source named "taken" is already registered' } },
      false,
    ))
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url).endsWith('/rename')) return post(url, init)
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse(detail)
    })

    renderDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    await user.click(screen.getByRole('button', { name: 'Rename' }))
    const dialog = await screen.findByRole('dialog')
    const input = within(dialog).getByRole('textbox')
    await user.clear(input)
    await user.type(input, 'taken')
    await user.click(within(dialog).getByRole('button', { name: 'Rename' }))

    expect(await within(dialog).findByText('a Source named "taken" is already registered')).toBeTruthy()
    expect(screen.getByRole('dialog', { name: 'Rename Source' })).toBeTruthy()
    expect(document.querySelector('h1')?.textContent).toBe('local-one')
  })
})
