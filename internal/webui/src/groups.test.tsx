import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { GroupsIndex, GroupExplorer } from './groups'
import { LocaleProvider } from './locale-provider'

import { setupMatchMedia } from './source-fixtures'

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

setupMatchMedia()

function mockResponse(data: any, ok = true, status = 200) {
  return new Response(JSON.stringify(data), {
    status: ok ? status : 400,
    headers: { 'Content-Type': 'application/json' },
  })
}

const mockGroupSummary = {
  id: 1,
  name: 'backend-tools',
  member_count: 2,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

const mockGroupView = {
  id: 1,
  name: 'backend-tools',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  members: [
    { id: 10, slug: 'go-lint', name: 'Go Linter' },
    { id: 11, slug: 'sql-check', name: 'SQL Checker' },
  ],
  targets: [{ id: 100, name: 'Dev Server' }],
}

const mockSkills = {
  items: [
    { id: 10, slug: 'go-lint', name: 'Go Linter' },
    { id: 11, slug: 'sql-check', name: 'SQL Checker' },
    { id: 12, slug: 'docker-fmt', name: 'Docker Formatter' },
  ],
  total: 3,
}

function renderGroups(initialEntry = '/groups') {
  return render(
    <LocaleProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/groups" element={<GroupsIndex />} />
          <Route path="/groups/:name" element={<GroupExplorer />} />
        </Routes>
      </MemoryRouter>
    </LocaleProvider>,
  )
}

describe('Groups UI', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/groups') {
        return mockResponse({ items: [mockGroupSummary], total: 1 })
      }
      if (u === '/api/v1/groups/1') {
        return mockResponse(mockGroupView)
      }
      if (u === '/api/v1/skills') {
        return mockResponse(mockSkills)
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })
  })

  afterEach(() => cleanup())

  it('renders groups list with member count and links', async () => {
    renderGroups()
    expect(await screen.findByRole('link', { name: 'backend-tools' })).toBeTruthy()
    expect(screen.getByText('2')).toBeTruthy()
  })

  it('submits create group form and calls POST /api/v1/groups', async () => {
    const user = userEvent.setup()
    renderGroups()
    await screen.findByRole('link', { name: 'backend-tools' })

    const postCall = vi.fn().mockResolvedValue(
      mockResponse({
        id: 2,
        name: 'frontend-tools',
        created_at: '2026-01-03T00:00:00Z',
        updated_at: '2026-01-03T00:00:00Z',
      }, true, 201),
    )

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url) === '/api/v1/groups') {
        return postCall(url, init)
      }
      if (String(url) === '/api/v1/groups') {
        return mockResponse({
          items: [
            mockGroupSummary,
            { id: 2, name: 'frontend-tools', member_count: 0, created_at: '2026-01-03T00:00:00Z', updated_at: '2026-01-03T00:00:00Z' },
          ],
          total: 2,
        })
      }
      return mockResponse({ items: [], total: 0 })
    })

    const input = screen.getByPlaceholderText('e.g. backend-dev-tools')
    await user.type(input, 'frontend-tools')
    await user.click(screen.getByRole('button', { name: 'Create Group' }))

    await waitFor(() => expect(postCall).toHaveBeenCalled())
    const [, init] = postCall.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({ name: 'frontend-tools' })
  })

  it('renders group detail page with members and targets by stable name route', async () => {
    renderGroups('/groups/backend-tools')
    expect(await screen.findByRole('heading', { name: 'backend-tools' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Go Linter' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'SQL Checker' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Dev Server' })).toBeTruthy()
  })

  it('removes member when remove button is clicked and sends Content-Type application/json', async () => {
    const user = userEvent.setup()
    renderGroups('/groups/backend-tools')
    await screen.findByRole('heading', { name: 'backend-tools' })

    const updatedGroupView = {
      ...mockGroupView,
      members: [{ id: 11, slug: 'sql-check', name: 'SQL Checker' }],
    }

    const deleteCall = vi.fn().mockResolvedValue(mockResponse(updatedGroupView))

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE' && String(url) === '/api/v1/groups/1/members/10') {
        return deleteCall(url, init)
      }
      if (String(url) === '/api/v1/groups') return mockResponse({ items: [mockGroupSummary], total: 1 })
      if (String(url) === '/api/v1/groups/1') return mockResponse(mockGroupView)
      if (String(url) === '/api/v1/skills') return mockResponse(mockSkills)
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    const removeBtn = screen.getByRole('button', { name: 'Remove Go Linter from group' })
    await user.click(removeBtn)

    await waitFor(() => expect(deleteCall).toHaveBeenCalled())
    const deleteInit = deleteCall.mock.calls[0][1] as RequestInit
    expect(deleteInit.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(screen.queryByRole('link', { name: 'Go Linter' })).toBeNull()
  })

  it('shows error state for non-existent group name', async () => {
    renderGroups('/groups/nonexistent')
    expect(await screen.findByText('Could not load Group')).toBeTruthy()
  })

  it('handles group names with % characters without double decoding or URIError', async () => {
    const specialGroupSummary = {
      id: 5,
      name: 'ops%20team',
      member_count: 1,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }
    const specialGroupView = {
      ...mockGroupView,
      id: 5,
      name: 'ops%20team',
    }

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/groups') return mockResponse({ items: [specialGroupSummary], total: 1 })
      if (u === '/api/v1/groups/5') return mockResponse(specialGroupView)
      if (u === '/api/v1/skills') return mockResponse(mockSkills)
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderGroups('/groups/ops%2520team')
    expect(await screen.findByRole('heading', { name: 'ops%20team' })).toBeTruthy()
  })
})
