import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SkillsIndex, SkillDetailPage, SkillExplorer } from './skills'
import { setupMatchMedia, mockResponse } from './source-fixtures'
import { skillAlpha, skillBeta } from './skill-fixtures'

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

describe('Skills Components', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const urlStr = String(url)
      if (urlStr === '/api/v1/skills') {
        return mockResponse({ items: [skillAlpha, skillBeta], total: 2 })
      }
      if (urlStr === '/api/v1/skills/101') {
        return mockResponse(skillAlpha)
      }
      if (urlStr === '/api/v1/skills/102') {
        return mockResponse(skillBeta)
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
  })

  afterEach(() => cleanup())

  it('renders SkillsIndex table with skills and links', async () => {
    render(
      <MemoryRouter initialEntries={['/skills']}>
        <Routes>
          <Route path="/skills" element={<SkillsIndex />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Skills' })).toBeTruthy()
    expect(screen.getByRole('table', { name: 'Skill Store' })).toBeTruthy()
    expect(screen.getByText('Alpha Skill')).toBeTruthy()
    expect(screen.getByText('Beta Skill')).toBeTruthy()
    expect(screen.getByText('local-one')).toBeTruthy()
  })

  it('renders SkillDetailPage with Overview tab and disabled Sync/Distribution tabs', async () => {
    render(
      <MemoryRouter initialEntries={['/skills/alpha']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    expect(screen.getByText('First test skill in local store')).toBeTruthy()

    // Tabs
    const overviewTab = screen.getByRole('tab', { name: 'Overview' })
    const syncTab = screen.getByRole('tab', { name: /Synchronization/ })
    const distTab = screen.getByRole('tab', { name: /Distribution/ })

    expect(overviewTab.getAttribute('aria-selected')).toBe('true')
    expect(syncTab.hasAttribute('data-disabled') || syncTab.hasAttribute('disabled') || syncTab.getAttribute('aria-disabled') === 'true').toBe(true)
    expect(distTab.hasAttribute('data-disabled') || distTab.hasAttribute('disabled') || distTab.getAttribute('aria-disabled') === 'true').toBe(true)

    // Binding info
    expect(screen.getByText('Source Binding')).toBeTruthy()
    expect(screen.getByText('local-one')).toBeTruthy()
    expect(screen.getByText('skills/alpha')).toBeTruthy()
  })

  it('handles unbound skill detail correctly', async () => {
    render(
      <MemoryRouter initialEntries={['/skills/beta']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Beta Skill' })).toBeTruthy()
    expect(screen.getByText(/This Skill is unbound/)).toBeTruthy()
  })

  it('renders master-detail SkillExplorer and supports fresh route behavior', async () => {
    render(
      <MemoryRouter initialEntries={['/skills/alpha']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillExplorer />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    expect(screen.getByRole('navigation', { name: 'Skill list' })).toBeTruthy()
  })

  it('shows error state when fetching skill fails', async () => {
    window.fetch = vi.fn().mockResolvedValue(mockResponse({ error: { message: 'Database query failed' } }, false, 500))

    render(
      <MemoryRouter initialEntries={['/skills/unknown']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByText('Could not load Skill')).toBeTruthy()
    expect(screen.getByText('Database query failed')).toBeTruthy()
  })
})
