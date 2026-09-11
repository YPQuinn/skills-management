import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SkillsIndex, SkillDetailPage, SkillExplorer } from './skills'
import { setupMatchMedia, mockResponse } from './source-fixtures'
import { skillAlpha, skillAlphaContent, skillBeta, skillBetaContent } from './skill-fixtures'
import { conflictSkill } from './skill-sync-fixtures'
import type { Skill } from './skill-api'

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
      if (urlStr === '/api/v1/skills/101/content') {
        return mockResponse(skillAlphaContent)
      }
      if (urlStr === '/api/v1/skills/102/content') {
        return mockResponse(skillBetaContent)
      }
      if (urlStr === '/api/v1/skills/101/diff') {
        return mockResponse({
          skill_id: 101,
          slug: 'alpha',
          source_digest: 'src',
          store_digest: 'store',
          baseline_digest: 'base',
          comparisons: [],
        })
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
    expect(screen.getByText('In sync')).toBeTruthy()
    expect(screen.getByText('Unbound')).toBeTruthy()
    for (const name of ['Alpha Skill', 'Beta Skill']) {
      const cell = screen.getByRole('link', { name }).closest('td')
      expect(cell?.querySelector('svg')).toBeTruthy()
    }
  })

  it('hides the slug when it matches the Skill name', async () => {
    renderSkillsIndex([{ ...skillAlpha, name: 'alpha', slug: 'alpha' }])
    const link = await screen.findByRole('link', { name: 'alpha' })
    expect(link.textContent).toBe('alpha')
  })

  it('keeps skill names on one line and clamps descriptions to two lines', async () => {
    const longName = 'codebase-design-and-architecture-review'
    const longDescription =
      'Ask which skill or flow fits your situation. A router over the skills in this repo, with enough extra detail that the cell must wrap beyond two lines and then ellipsize.'
    renderSkillsIndex([{ ...skillAlpha, name: longName, slug: longName, description: longDescription }])

    const nameLink = await screen.findByRole('link', { name: longName })
    expect(nameLink.className).toMatch(/whitespace-nowrap/)
    expect(nameLink.closest('td')?.className).toMatch(/whitespace-nowrap/)

    const description = screen.getByText(longDescription)
    expect(description.className).toMatch(/line-clamp-2/)
    expect(description.closest('td')?.className).toMatch(/max-w-0/)
  })

  it('renders SkillDetailPage with Overview and Synchronization tabs', async () => {
    render(
      <MemoryRouter initialEntries={['/skills/alpha']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()

    const overviewTab = screen.getByRole('tab', { name: 'Overview' })
    const syncTab = screen.getByRole('tab', { name: 'Synchronization' })
    expect(overviewTab.getAttribute('aria-selected')).toBe('true')
    expect(overviewTab.className).not.toMatch(/!text-black/)
    expect(syncTab.className).not.toMatch(/!text-black/)
    expect(screen.queryByRole('tab', { name: 'Distribution' })).toBeNull()

    // The Overview tab renders the Skill's own SKILL.md.
    expect(await screen.findByRole('heading', { name: 'Attributes' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Alpha', level: 2 })).toBeTruthy()
    expect(screen.getByText('npm test')).toBeTruthy()
    // The description now reaches the page only through the frontmatter value.
    expect(screen.getAllByText('First test skill in local store')).toHaveLength(1)
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
    expect(screen.getByText('unbound')).toBeTruthy()
    // The Overview tab still renders the unbound Skill's own document.
    expect(await screen.findByRole('heading', { name: 'Beta', level: 2 })).toBeTruthy()
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
    expect(screen.getAllByText('In sync').length).toBeGreaterThan(0)
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

function makeSkills(count: number, nameFor?: (index: number) => string): Skill[] {
  return Array.from({ length: count }, (_, index) => ({
    id: 200 + index,
    slug: `slug-${200 + index}`,
    name: nameFor ? nameFor(index) : `Generated Skill ${index + 1}`,
    description: `Generated description ${index + 1}`,
    store_digest: 'sha256:0000000000000000000000000000000000000000000000000000000000000000',
    baseline_digest: 'sha256:0000000000000000000000000000000000000000000000000000000000000000',
    created_at: '2026-08-11T10:00:00Z',
    updated_at: '2026-08-11T10:00:00Z',
    sync_status: 'in_sync',
    sync_stale: false,
    has_previous_snapshot: false,
  }))
}

function renderSkillsIndex(skills: Skill[]) {
  window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
    if (String(url) === '/api/v1/skills') {
      return mockResponse({ items: skills, total: skills.length })
    }
    return mockResponse({ error: { message: 'Not found' } }, false, 404)
  })
  render(
    <MemoryRouter initialEntries={['/skills']}>
      <Routes>
        <Route path="/skills" element={<SkillsIndex />} />
      </Routes>
    </MemoryRouter>,
  )
}

function currentPageNumber(): string {
  return document.querySelector('[aria-current="page"]')?.textContent?.trim() ?? ''
}

describe('SkillsIndex pagination and search', () => {
  afterEach(() => cleanup())

  it('filters the table by Sync Status chip', async () => {
    renderSkillsIndex([skillAlpha, skillBeta, conflictSkill])
    expect(await screen.findByText('Alpha Skill')).toBeTruthy()
    expect(screen.getByText('Beta Skill')).toBeTruthy()
    expect(screen.getByText('Conflict Skill')).toBeTruthy()
    expect(screen.getByRole('button', { name: '1 conflict' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '0 Source changed' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '1 unbound' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '1 conflict' }))

    expect(screen.getByText('Conflict Skill')).toBeTruthy()
    expect(screen.queryByText('Alpha Skill')).toBeNull()
    expect(screen.queryByText('Beta Skill')).toBeNull()
    expect(screen.getByRole('button', { name: '1 conflict' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('paginates 10 skills per page and switches pages', async () => {
    renderSkillsIndex(makeSkills(12))
    expect(await screen.findByText('Generated Skill 1')).toBeTruthy()
    expect(screen.getByText('Generated Skill 10')).toBeTruthy()
    expect(screen.queryByText('Generated Skill 11')).toBeNull()
    expect(screen.queryByText('Generated Skill 12')).toBeNull()
    expect(screen.getByRole('navigation', { name: 'Pagination' })).toBeTruthy()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))

    expect(screen.getByText('Generated Skill 11')).toBeTruthy()
    expect(screen.getByText('Generated Skill 12')).toBeTruthy()
    expect(screen.queryByText('Generated Skill 1')).toBeNull()
  })

  it('filters by case-insensitive, trimmed name substring', async () => {
    renderSkillsIndex([
      ...makeSkills(1, () => 'React Hooks Guide'),
      ...makeSkills(1, () => 'react router basics'),
      ...makeSkills(1, () => 'Testing Patterns'),
    ])
    await screen.findByText('React Hooks Guide')

    fireEvent.change(screen.getByLabelText('Search Skills by name'), { target: { value: '  ReAcT  ' } })

    expect(screen.getByText('React Hooks Guide')).toBeTruthy()
    expect(screen.getByText('react router basics')).toBeTruthy()
    expect(screen.queryByText('Testing Patterns')).toBeNull()
  })

  it('returns to page 1 when searching from a later page and when clearing', async () => {
    renderSkillsIndex(makeSkills(12, (index) => `Guide Skill ${index + 1}`))
    await screen.findByText('Guide Skill 1')

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(currentPageNumber()).toBe('2')

    fireEvent.change(screen.getByLabelText('Search Skills by name'), { target: { value: 'guide' } })
    expect(currentPageNumber()).toBe('1')
    expect(screen.getByText('Guide Skill 1')).toBeTruthy()
    expect(screen.queryByText('Guide Skill 12')).toBeNull()

    // Move to page 2 of the filtered results, then clear: back to page 1 of the full list.
    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(currentPageNumber()).toBe('2')

    fireEvent.click(screen.getByRole('button', { name: 'Clear input' }))
    expect(currentPageNumber()).toBe('1')
    expect(screen.getByText('Guide Skill 1')).toBeTruthy()
  })

  it('shows a localized no-results state and keeps the search box visible', async () => {
    renderSkillsIndex(makeSkills(12))
    await screen.findByText('Generated Skill 1')

    fireEvent.change(screen.getByLabelText('Search Skills by name'), { target: { value: 'zzz-no-match' } })

    expect(screen.getByText('No Skills match your search.')).toBeTruthy()
    expect(screen.getByLabelText('Search Skills by name')).toBeTruthy()
    expect(screen.queryByRole('table', { name: 'Skill Store' })).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })

  it('disables prev/next links at the bounds', async () => {
    renderSkillsIndex(makeSkills(12))
    await screen.findByText('Generated Skill 1')

    expect(screen.getByRole('link', { name: 'Go to previous page' }).hasAttribute('aria-disabled')).toBe(true)
    expect(screen.getByRole('link', { name: 'Go to next page' }).hasAttribute('aria-disabled')).toBe(false)

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(screen.getByRole('link', { name: 'Go to previous page' }).hasAttribute('aria-disabled')).toBe(false)
    expect(screen.getByRole('link', { name: 'Go to next page' }).hasAttribute('aria-disabled')).toBe(true)
  })

  it('hides pagination when the skill list fits on one page', async () => {
    renderSkillsIndex(makeSkills(2))
    expect(await screen.findByText('Generated Skill 1')).toBeTruthy()
    expect(screen.getByText('Generated Skill 2')).toBeTruthy()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })

  it('collapses long page ranges with an ellipsis and a bounded number of page links', async () => {
    renderSkillsIndex(makeSkills(125)) // 13 pages
    await screen.findByText('Generated Skill 1')

    const nav = screen.getByRole('navigation', { name: 'Pagination' })
    expect(nav.querySelectorAll('[data-slot="pagination-ellipsis"]').length).toBeGreaterThan(0)
    // Only first/last pages plus the current page and its neighbours are rendered.
    expect(screen.getAllByRole('link', { name: /^Go to page \d+$/ }).length).toBeLessThanOrEqual(5)

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 13' }))
    expect(currentPageNumber()).toBe('13')
    expect(screen.getByRole('link', { name: 'Go to page 1' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Go to page 12' })).toBeTruthy()
    expect(nav.querySelectorAll('[data-slot="pagination-ellipsis"]').length).toBeGreaterThan(0)
  })

  it('keeps the current page non-interactive and out of the tab order', async () => {
    renderSkillsIndex(makeSkills(125))
    await screen.findByText('Generated Skill 1')

    const active = document.querySelector('[aria-current="page"]')
    expect(active?.textContent?.trim()).toBe('1')
    // The current page is not a link destination and carries no go-to label.
    expect(active?.hasAttribute('href')).toBe(false)
    expect(screen.queryByRole('link', { name: 'Go to page 1' })).toBeNull()
    // Narrow keyboard semantics: not reachable via Tab.
    expect(active?.getAttribute('tabindex')).toBe('-1')
    const prev = screen.getByRole('link', { name: 'Go to previous page' })
    expect(prev.getAttribute('aria-disabled')).toBe('true')
    expect(prev.getAttribute('tabindex')).toBe('-1')
  })

  it('keeps the empty-Store CTA and shows no search or no-results UI for an empty store', async () => {
    renderSkillsIndex([])

    expect(await screen.findByText('No Skills in local Store yet.')).toBeTruthy()
    expect(screen.getByText('Register a Source and import Skills to get started.')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Go to Sources' })).toBeTruthy()
    expect(screen.queryByLabelText('Search Skills by name')).toBeNull()
    expect(screen.queryByText('No Skills match your search.')).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })
})
