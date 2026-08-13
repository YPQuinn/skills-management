import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { TargetsIndex, TargetExplorer } from './targets'
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

const mockAdapters = [
  {
    key: 'claude',
    name: 'Claude Code',
    detection: {
      status: 'detected',
      evidence: ['found /usr/local/bin/claude'],
      detected_at: '2026-01-01T00:00:00Z',
    },
  },
]

const mockTargetSummary = {
  id: 1,
  name: 'Claude User Config',
  path: '/Users/test/.claude/skills',
  adapter: 'claude',
  scope: 'user',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const mockTargetView = {
  ...mockTargetSummary,
  compatible_adapters: ['claude'],
  direct_skills: [
    {
      id: 101,
      kind: 'skill',
      skill: { id: 10, slug: 'go-lint', name: 'Go Linter' },
      created_at: '2026-01-01T00:00:00Z',
    },
  ],
  groups: [
    {
      id: 102,
      kind: 'group',
      group: { id: 1, name: 'backend-tools' },
      created_at: '2026-01-01T00:00:00Z',
    },
  ],
  desired_skills: [
    {
      id: 10,
      slug: 'go-lint',
      name: 'Go Linter',
      reasons: [{ assignment_id: 101, kind: 'skill' }],
    },
    {
      id: 11,
      slug: 'sql-check',
      name: 'SQL Checker',
      reasons: [{ assignment_id: 102, kind: 'group', group_id: 1, group_name: 'backend-tools' }],
    },
  ],
}

function renderTargets(initialEntry = '/targets') {
  return render(
    <LocaleProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/targets" element={<TargetsIndex />} />
          <Route path="/targets/:name" element={<TargetExplorer />} />
        </Routes>
      </MemoryRouter>
    </LocaleProvider>,
  )
}

describe('Targets UI', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/targets/adapters') {
        return mockResponse({ items: mockAdapters, total: 1 })
      }
      if (u === '/api/v1/targets') {
        return mockResponse({ items: [mockTargetSummary], total: 1 })
      }
      if (u === '/api/v1/targets/1') {
        return mockResponse(mockTargetView)
      }
      if (u === '/api/v1/skills') {
        return mockResponse({ items: [{ id: 10, slug: 'go-lint', name: 'Go Linter' }], total: 1 })
      }
      if (u === '/api/v1/groups') {
        return mockResponse({ items: [{ id: 1, name: 'backend-tools', member_count: 2 }], total: 1 })
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })
  })

  afterEach(() => cleanup())

  it('renders target adapters with detection evidence and registered targets', async () => {
    renderTargets()
    expect(await screen.findByText('Claude Code')).toBeTruthy()
    expect(screen.getByText('Detected')).toBeTruthy()
    expect(screen.getByText('found /usr/local/bin/claude')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Claude User Config' })).toBeTruthy()
  })

  it('registers a custom target via POST /api/v1/targets without adapter/scope/project_root', async () => {
    const user = userEvent.setup()
    renderTargets()
    await screen.findByText('Claude Code')

    const postCall = vi.fn().mockResolvedValue(
      mockResponse({
        id: 2,
        name: 'Custom Agent Skills',
        path: '/tmp/agent-skills',
        adapter: 'custom',
        scope: 'custom',
        created_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
        direct_skills: [],
        groups: [],
        desired_skills: [],
      }, true, 201),
    )

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url) === '/api/v1/targets') {
        return postCall(url, init)
      }
      if (String(url) === '/api/v1/targets/adapters') return mockResponse({ items: mockAdapters, total: 1 })
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    const modeSelect = screen.getByRole('combobox', { name: 'Target Type' })
    await user.click(modeSelect)
    const customOption = await screen.findByRole('option', { name: 'Custom Directory Path' })
    await user.click(customOption)

    const nameInput = screen.getByPlaceholderText('e.g. Claude User Config')
    await user.type(nameInput, 'Custom Agent Skills')

    const pathInput = screen.getByPlaceholderText('/path/to/target/skills')
    await user.type(pathInput, '/tmp/agent-skills')

    await user.click(screen.getByRole('button', { name: 'Register Target' }))

    await waitFor(() => expect(postCall).toHaveBeenCalled())
    const [, init] = postCall.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({
      name: 'Custom Agent Skills',
      path: '/tmp/agent-skills',
    })
  })

  it('renders target detail page by stable name route with direct assignments and expanded desired set reasons', async () => {
    renderTargets('/targets/Claude%20User%20Config')
    expect(await screen.findByRole('heading', { name: 'Claude User Config' })).toBeTruthy()
    expect(screen.getAllByRole('link', { name: 'Go Linter' }).length).toBe(2)
    expect(screen.getByRole('link', { name: 'backend-tools' })).toBeTruthy()

    // Desired skills section
    expect(screen.getByText('Desired Skill Set (2)')).toBeTruthy()
    expect(screen.getByText('Directly assigned (Assignment #101)')).toBeTruthy()
    expect(screen.getByText('Assigned via Group "backend-tools" (Group #1, Assignment #102)')).toBeTruthy()
  })

  it('deletes assignment via DELETE /api/v1/targets/1/assignments/:id with Content-Type application/json', async () => {
    const user = userEvent.setup()
    renderTargets('/targets/Claude%20User%20Config')
    await screen.findByRole('heading', { name: 'Claude User Config' })

    const updatedTargetView = {
      ...mockTargetView,
      direct_skills: [],
    }

    const deleteCall = vi.fn().mockResolvedValue(mockResponse(updatedTargetView))

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE' && String(url) === '/api/v1/targets/1/assignments/101') {
        return deleteCall(url, init)
      }
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      if (String(url) === '/api/v1/targets/1') return mockResponse(mockTargetView)
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      if (String(url) === '/api/v1/groups') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    const unassignBtn = screen.getByRole('button', { name: 'Unassign Go Linter from target' })
    await user.click(unassignBtn)

    await waitFor(() => expect(deleteCall).toHaveBeenCalled())
    const deleteInit = deleteCall.mock.calls[0][1] as RequestInit
    expect(deleteInit.headers).toEqual({ 'Content-Type': 'application/json' })
  })

  it('shows error state for non-existent target name', async () => {
    renderTargets('/targets/nonexistent')
    expect(await screen.findByText('Could not load Target')).toBeTruthy()
  })

  it('renders all 4 adapter detection statuses (detected, not_detected, unknown, not_applicable)', async () => {
    const multiStatusAdapters = [
      { key: 'a1', name: 'Adapter 1', detection: { status: 'detected', evidence: ['e1'], detected_at: '2026-01-01T00:00:00Z' } },
      { key: 'a2', name: 'Adapter 2', detection: { status: 'not_detected', evidence: [], detected_at: '2026-01-01T00:00:00Z' } },
      { key: 'a3', name: 'Adapter 3', detection: { status: 'unknown', evidence: [], detected_at: '2026-01-01T00:00:00Z' } },
      { key: 'a4', name: 'Adapter 4', detection: { status: 'not_applicable', evidence: [], detected_at: '2026-01-01T00:00:00Z' } },
    ]

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/targets/adapters') return mockResponse({ items: multiStatusAdapters, total: 4 })
      if (u === '/api/v1/targets') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderTargets('/targets')
    expect(await screen.findByText('Detected')).toBeTruthy()
    expect(screen.getByText('Not Detected')).toBeTruthy()
    expect(screen.getByText('Unknown')).toBeTruthy()
    expect(screen.getByText('N/A')).toBeTruthy()
  })

  it('handles target names with % characters without double decoding or URIError', async () => {
    const specialTargetSummary = {
      id: 99,
      name: '100% coverage',
      path: '/tmp/coverage',
      adapter: 'custom',
      scope: 'custom',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }
    const specialTargetView = {
      ...mockTargetView,
      id: 99,
      name: '100% coverage',
    }

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/targets') return mockResponse({ items: [specialTargetSummary], total: 1 })
      if (u === '/api/v1/targets/99') return mockResponse(specialTargetView)
      if (u === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      if (u === '/api/v1/groups') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderTargets('/targets/100%25%20coverage')
    expect(await screen.findByRole('heading', { name: '100% coverage' })).toBeTruthy()
  })
})
