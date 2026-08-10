import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
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

  it('renders a compact Source list beside the detail without the registration form or six-column table', async () => {
    renderExplorer('/sources/local-one')
    const pane = within(await screen.findByRole('navigation', { name: 'Source list' }))

    // concise context: name, status, entry count, and location
    expect(pane.getByRole('link', { name: /local-one/ })).toBeTruthy()
    expect(pane.getByText('Available')).toBeTruthy()
    expect(pane.getByText('2 skills')).toBeTruthy()
    expect(pane.getByText('/tmp/skills')).toBeTruthy()

    // the compact pane never duplicates the registration form or the full table
    expect(screen.queryByRole('button', { name: /Register and scan/ })).toBeNull()
    expect(screen.queryByRole('columnheader', { name: 'Last checked' })).toBeNull()

    // the selected Source detail is the master–detail partner for the deep link
    expect(await screen.findByRole('heading', { name: 'local-one' })).toBeTruthy()
  })

  it('never shows the previous Source while a newer route loads and ignores stale responses', async () => {
    const user = userEvent.setup()
    const summaryOne = { ...summary, id: 1, name: 'source-one' }
    const summaryTwo = { ...summary, id: 2, name: 'source-two', location: '/tmp/two' }
    const detailOne = { ...detail, id: 1, name: 'source-one' }
    const detailTwo = { ...detail, id: 2, name: 'source-two', location: '/tmp/two' }
    const pending: Record<string, (value: Response) => void> = {}

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/sources') return mockResponse({ items: [summaryOne, summaryTwo], total: 2 })
      if (u === '/api/v1/sources/1' || u === '/api/v1/sources/2') {
        return new Promise<Response>((resolve) => { pending[u] = resolve })
      }
      return mockResponse({ error: { message: 'unexpected call' } }, false)
    })

    renderExplorer('/sources/source-one')
    await screen.findByRole('link', { name: /source-one/ })

    // switch to Source 2 while Source 1's detail is still in flight
    await user.click(screen.getByRole('link', { name: /source-two/ }))

    // the previous Source must not remain on screen as the new selection
    expect(screen.queryByRole('heading', { name: 'source-one' })).toBeNull()
    expect(screen.getByLabelText('Loading source')).toBeTruthy()

    // the current route's response lands first
    pending['/api/v1/sources/2'](mockResponse(detailTwo))
    expect(await screen.findByRole('heading', { name: 'source-two' })).toBeTruthy()

    // the stale response for the previous route lands later and must not win
    pending['/api/v1/sources/1'](mockResponse(detailOne))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'source-two' })).toBeTruthy())
    expect(screen.queryByRole('heading', { name: 'source-one' })).toBeNull()
  })

  it('shows the compact pane empty state with a Register link and fails to load detail when name cannot be resolved', async () => {
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/sources') return mockResponse({ items: [], total: 0 })
      return mockResponse(detail)
    })
    renderExplorer('/sources/local-one')

    const pane = within(await screen.findByRole('navigation', { name: 'Source list' }))
    expect(pane.getByText('No Sources registered yet.')).toBeTruthy()
    const register = pane.getByRole('link', { name: 'Register a Source' })
    expect(register.getAttribute('href')).toBe('/sources')

    expect(await screen.findByText('Source not found')).toBeTruthy()
  })
})
