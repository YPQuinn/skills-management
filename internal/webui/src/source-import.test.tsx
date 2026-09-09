import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SourceDetailPage } from './source-detail'
import { setupMatchMedia, summary, detail, mockResponse } from './source-fixtures'
import { skillAlpha, mockImportSuccess, mockImportReplaced } from './skill-fixtures'

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

function renderSourceDetail(initialRoute = '/sources/local-one') {
  return render(
    <MemoryRouter initialEntries={[initialRoute]}>
      <Routes>
        <Route path="/sources/:name" element={<SourceDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Source Import & Conflict Replace UI', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const urlStr = String(url)
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [skillAlpha], total: 1 })
      return mockResponse({ error: { message: 'Unknown endpoint' } }, false, 404)
    })
  })

  afterEach(() => cleanup())

  it('renders accessible TableCaption for Source inventory table', async () => {
    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })
    expect(screen.getByRole('table', { name: 'Source inventory' })).toBeTruthy()
  })

  it('imports all inventory items and displays summary', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        const body = JSON.parse(String(init.body))
        expect(body.source_id).toBe(1)
        expect(body.all).toBe(true)
        return mockResponse(mockImportSuccess)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Import all' }))

    expect(await screen.findByText(/Import completed/)).toBeTruthy()
    expect(screen.getByText(/Total: 1, 1 imported/)).toBeTruthy()
  })

  it('imports selected items with slug override saved from the dialog', async () => {
    let importBody: any
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        importBody = JSON.parse(String(init.body))
        return mockResponse(mockImportSuccess)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const user = userEvent.setup()
    await user.click(screen.getByRole('checkbox', { name: 'Select Alpha' }))

    // open the low-key trigger and edit inside the Dialog
    await user.click(screen.getByRole('button', { name: 'Slug override for Alpha (skills/alpha)' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByRole('textbox'), 'alpha-custom')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.getByRole('button', { name: 'Slug override for Alpha (skills/alpha)' }).textContent).toContain('alpha-custom')

    await user.click(screen.getByRole('button', { name: /Import selected/ }))

    expect(importBody).toBeDefined()
    expect(importBody.selectors).toEqual([{ relative_dir: 'skills/alpha', slug: 'alpha-custom' }])
  })

  it('does not apply a slug override cancelled in the dialog', async () => {
    let importBody: any
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        importBody = JSON.parse(String(init.body))
        return mockResponse(mockImportSuccess)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const user = userEvent.setup()
    await user.click(screen.getByRole('checkbox', { name: 'Select Alpha' }))

    await user.click(screen.getByRole('button', { name: 'Slug override for Alpha (skills/alpha)' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByRole('textbox'), 'alpha-temp')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // trigger still shows the default state and nothing was saved
    expect(screen.getByRole('button', { name: 'Slug override for Alpha (skills/alpha)' }).textContent).toContain('Default')

    await user.click(screen.getByRole('button', { name: /Import selected/ }))

    expect(importBody).toBeDefined()
    expect(importBody.selectors).toEqual([{ relative_dir: 'skills/alpha' }])
  })

  it('shows error alert on import failure', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        return mockResponse({ error: { message: 'Database transaction failed' } }, false, 500)
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Import all' }))

    expect(await screen.findByText('Import failed')).toBeTruthy()
    expect(screen.getByText('Database transaction failed')).toBeTruthy()
  })

  it('aborts replace request signal on unmount and Check again clears stale dialog/result', async () => {
    let replaceSignal!: AbortSignal
    let replaceResolve!: (res: Response) => void
    const replacePromise = new Promise<Response>((r) => {
      replaceResolve = r
    })

    const conflictRes = {
      items: [
        {
          status: 'skipped_conflict',
          relative_dir: 'skills/alpha',
          replaces: { skill_id: 101, slug: 'alpha', name: 'Alpha Skill' },
        },
      ],
      summary: { total: 1, imported: 0, already_imported: 0, skipped_conflict: 1, replaced: 0, failed: 0 },
    }

    let importPostCount = 0

    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (init?.method === 'POST' && urlStr === '/api/v1/skills/import') {
        importPostCount++
        if (importPostCount === 1) return mockResponse(conflictRes)
        replaceSignal = init.signal as AbortSignal
        return replacePromise
      }
      if (urlStr === '/api/v1/sources') return mockResponse({ items: [summary], total: 1 })
      if (urlStr === '/api/v1/sources/1') return mockResponse(detail)
      if (urlStr === '/api/v1/sources/1/check') return mockResponse(detail)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })

    renderSourceDetail()
    await screen.findByRole('heading', { name: 'local-one' })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Import all' }))

    expect(await screen.findByText('Replace existing Skill?')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Replace Skill' }))

    expect(replaceSignal).toBeDefined()
    expect(replaceSignal.aborted).toBe(false)

    await user.click(screen.getByRole('button', { name: 'Rescan', hidden: true }))

    expect(replaceSignal.aborted).toBe(true)
    await waitFor(() => {
      expect(screen.queryByText('Replace existing Skill?')).toBeNull()
    })

    replaceResolve(mockResponse(mockImportReplaced))
    await new Promise((r) => setTimeout(r, 10))
  })
})
