import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourceExplorer, SourcesIndex } from './sources'
import { setupMatchMedia, summary, detail, mockResponse } from './source-fixtures'

setupMatchMedia()

function renderExplorer(initialPath = '/sources/local-one') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/sources" element={<SourcesIndex />} />
        <Route path="/sources/:name" element={<SourceExplorer />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('SourceExplorer', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      return mockResponse(detail)
    })
  })

  afterEach(() => cleanup())

  it('renders the Source detail full-width without a Source list pane or the registration form', async () => {
    renderExplorer('/sources/local-one')

    expect(await screen.findByRole('heading', { name: 'local-one' })).toBeTruthy()
    expect(screen.getByText(/Inventory \(2\)/)).toBeTruthy()

    // the compact source list pane is gone and the detail owns the content area
    expect(screen.queryByRole('navigation', { name: 'Source list' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Register and scan/ })).toBeNull()
  })

  it('switches between Sources through the index without showing the previous Source', async () => {
    const user = userEvent.setup()
    const summaryTwo = { ...summary, id: 2, name: 'source-two', location: '/tmp/two' }
    const detailTwo = { ...detail, id: 2, name: 'source-two', location: '/tmp/two' }
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/sources') return mockResponse({ items: [summary, summaryTwo], total: 2 })
      if (u === '/api/v1/sources/1') return mockResponse(detail)
      if (u === '/api/v1/sources/2') return mockResponse(detailTwo)
      return mockResponse({ error: { message: 'unexpected call' } }, false)
    })

    renderExplorer('/sources/local-one')
    expect(await screen.findByRole('heading', { name: 'local-one' })).toBeTruthy()

    // back to the index, then into the other Source
    await user.click(screen.getByRole('link', { name: 'All Sources' }))
    const indexTable = await screen.findByRole('table', { name: 'Registered Sources' })
    await user.click(within(indexTable).getByRole('link', { name: 'source-two' }))

    expect(await screen.findByRole('heading', { name: 'source-two' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'local-one' })).toBeNull()
  })

  it('fails to load detail when name cannot be resolved', async () => {
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [], total: 0 })
      return mockResponse(detail)
    })
    renderExplorer('/sources/local-one')

    expect(await screen.findByText('Source not found')).toBeTruthy()
  })
})
