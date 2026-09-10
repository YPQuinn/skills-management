import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
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
      reasons: [
        { assignment_id: 101, kind: 'skill' },
        { assignment_id: 102, kind: 'group', group_id: 1, group_name: 'backend-tools' },
      ],
    },
    {
      id: 11,
      slug: 'sql-check',
      name: 'SQL Checker',
      reasons: [{ assignment_id: 102, kind: 'group', group_id: 1, group_name: 'backend-tools' }],
    },
  ],
}

const mockStatus = {
  target_id: 1,
  name: 'Claude User Config',
  path: '/Users/test/.claude/skills',
  state: 'missing',
  stale: false,
  items: [],
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
      if (u === '/api/v1/targets/1/inspect') {
        return mockResponse(mockStatus)
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

  it('registers targets from the form alone, without exposing detection evidence', async () => {
    const user = userEvent.setup()
    renderTargets()
    expect(await screen.findByRole('link', { name: 'Claude User Config' })).toBeTruthy()
    expect(screen.getByText('Never distributed')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Add Distribution Target' }))
    expect(await screen.findByRole('combobox', { name: 'Agent Type' })).toBeTruthy()
    expect(screen.queryByText('found /usr/local/bin/claude')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Register as Target' })).toBeNull()
  })

  it('shows last Distribution outcome and stale on registered Targets', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/targets/adapters') {
        return mockResponse({ items: mockAdapters, total: 1 })
      }
      if (u === '/api/v1/targets') {
        return mockResponse({
          items: [{ ...mockTargetSummary, last_result: 'blocked', stale: true }],
          total: 1,
        })
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })
    renderTargets()
    expect(await screen.findByText('Blocked')).toBeTruthy()
    expect(screen.getByText('stale observation')).toBeTruthy()
  })

  it('lists Agent Type and Scope as labels instead of the stored keys', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const u = String(url)
      if (u === '/api/v1/targets/adapters') {
        return mockResponse({ items: mockAdapters, total: 1 })
      }
      if (u === '/api/v1/targets') {
        return mockResponse({
          items: [
            mockTargetSummary,
            { ...mockTargetSummary, id: 2, name: 'Scratch Dir', adapter: 'custom', scope: 'custom' },
          ],
          total: 2,
        })
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })
    renderTargets()

    const table = await screen.findByRole('table', { name: 'Registered Targets' })
    const rows = within(table).getAllByRole('row')

    const builtin = within(rows[1]).getAllByRole('cell')
    expect(builtin[1].textContent).toBe('Claude Code')
    expect(builtin[2].textContent).toBe('User (global)')

    const custom = within(rows[2]).getAllByRole('cell')
    expect(custom[1].textContent).toBe('Custom Directory')
    expect(custom[2].textContent).toBe('Custom')
  })

  it('registers a custom target via POST /api/v1/targets without adapter/scope/project_root', async () => {
    const user = userEvent.setup()
    renderTargets()
    await screen.findByRole('link', { name: 'Claude User Config' })
    await user.click(screen.getByRole('button', { name: 'Add Distribution Target' }))

    const modeSelect = screen.getByRole('combobox', { name: 'Target Type' })
    expect(modeSelect.textContent).toContain('Popular Agent')
    expect(modeSelect.textContent).not.toMatch(/^\s*builtin\s*$/)
    const adapterSelect = screen.getByRole('combobox', { name: 'Agent Type' })
    expect(adapterSelect.textContent).toContain('Claude Code')
    expect(adapterSelect.textContent).not.toMatch(/^\s*claude\s*$/)
    expect(screen.getByRole('combobox', { name: 'Scope' }).textContent).toContain('User (global)')

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

    await user.click(modeSelect)
    const customOption = await screen.findByRole('option', { name: 'Custom Directory' })
    await user.click(customOption)
    expect(modeSelect.textContent).toContain('Custom Directory')
    expect(modeSelect.textContent).not.toMatch(/^\s*custom\s*$/)

    const nameInput = screen.getByPlaceholderText('e.g. Claude User Config')
    await user.type(nameInput, 'Custom Agent Skills')

    const pathInput = screen.getByPlaceholderText('/path/to/target/skills')
    await user.type(pathInput, '/tmp/agent-skills')

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Add' }))

    await waitFor(() => expect(postCall).toHaveBeenCalled())
    const [, init] = postCall.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({
      name: 'Custom Agent Skills',
      path: '/tmp/agent-skills',
    })
  })

  it('surfaces the server message under an Adding Target failed alert when the POST fails', async () => {
    const user = userEvent.setup()
    renderTargets()
    await screen.findByRole('link', { name: 'Claude User Config' })
    await user.click(screen.getByRole('button', { name: 'Add Distribution Target' }))

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url) === '/api/v1/targets') {
        return mockResponse({ error: { message: 'target name already taken' } }, false, 409)
      }
      if (String(url) === '/api/v1/targets/adapters') return mockResponse({ items: mockAdapters, total: 1 })
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    await user.type(screen.getByPlaceholderText('e.g. Claude User Config'), 'Claude User Config')
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Add' }))

    expect(await screen.findByText('Adding Target failed')).toBeTruthy()
    expect(screen.getByText('target name already taken')).toBeTruthy()
  })

  it('renders the merged Skill Assignments section with skill-level rows expanded from Groups', async () => {
    renderTargets('/targets/Claude%20User%20Config')
    expect(await screen.findByRole('heading', { name: 'Claude User Config' })).toBeTruthy()

    // Agent Type and Scope read as labels, not the keys the Target stores
    expect(await screen.findByText('Claude Code')).toBeTruthy()
    expect(screen.getByText('User (global)')).toBeTruthy()
    expect(screen.queryByText('claude')).toBeNull()

    // One merged section, headed by the direct-assignments count of desired Skills.
    expect(screen.getByRole('heading', { name: /Skill Assignments \(2\)/ })).toBeTruthy()
    // The old dual-block layout is gone.
    expect(screen.queryByRole('heading', { name: 'All Skills List' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Preview desired Skills' })).toBeNull()

    // Each desired Skill is a single row; the Group Assignment expands into its members.
    expect(screen.getAllByRole('link', { name: 'Go Linter' }).length).toBe(1)
    expect(screen.getByRole('link', { name: 'SQL Checker' })).toBeTruthy()
    // Both member Skills carry the originating Group chip as a link.
    expect(screen.getAllByRole('link', { name: 'backend-tools' }).length).toBe(2)
  })

  it('assigns a Group through the searchable picker', async () => {
    const user = userEvent.setup()
    const postCall = vi.fn().mockResolvedValue(
      mockResponse({
        id: 103,
        kind: 'group',
        group: { id: 2, name: 'crew' },
        created_at: '2026-01-01T00:00:00Z',
      }),
    )
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url) === '/api/v1/targets/1/assignments') {
        return postCall(url, init)
      }
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      if (String(url) === '/api/v1/targets/1') return mockResponse(mockTargetView)
      if (String(url) === '/api/v1/targets/1/inspect') return mockResponse(mockStatus)
      if (String(url) === '/api/v1/skills') {
        return mockResponse({ items: [{ id: 10, slug: 'go-lint', name: 'Go Linter' }], total: 1 })
      }
      if (String(url) === '/api/v1/groups') {
        return mockResponse({
          items: [
            { id: 1, name: 'backend-tools', member_count: 2 },
            { id: 2, name: 'crew', member_count: 1 },
          ],
          total: 2,
        })
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderTargets('/targets/Claude%20User%20Config')
    await screen.findByRole('heading', { name: 'Claude User Config' })
    await user.click(screen.getByRole('combobox', { name: 'Assignment Type' }))
    await user.click(await screen.findByRole('option', { name: 'Skill Group' }))
    expect(screen.getByRole('combobox', { name: 'Assignment Type' }).textContent).toContain('Skill Group')
    expect(screen.getByRole('combobox', { name: 'Assignment Type' }).textContent).not.toMatch(/^\s*group\s*$/)
    const picker = screen.getByRole('combobox', { name: 'Skill Group' })
    await user.click(picker)
    await user.click(await screen.findByRole('option', { name: 'crew' }))
    await user.click(screen.getByRole('button', { name: 'Assign' }))
    await waitFor(() => expect(postCall).toHaveBeenCalled())
    expect(JSON.parse((postCall.mock.calls[0][1] as RequestInit).body as string)).toEqual({
      kind: 'group',
      group_id: 2,
    })
  })

  it('shows an empty Group picker instead of an empty popup when none remain', async () => {
    const user = userEvent.setup()
    renderTargets('/targets/Claude%20User%20Config')
    await screen.findByRole('heading', { name: 'Claude User Config' })
    await user.click(screen.getByRole('combobox', { name: 'Assignment Type' }))
    await user.click(await screen.findByRole('option', { name: 'Skill Group' }))
    const picker = screen.getByRole('combobox', { name: 'Skill Group' })
    expect(picker.getAttribute('data-disabled')).not.toBeNull()
    expect(picker.textContent).toContain('No Groups left to assign')
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('assigns multiple Skills in one submit', async () => {
    const user = userEvent.setup()
    const postCall = vi.fn().mockResolvedValue(
      mockResponse({
        id: 200,
        kind: 'skill',
        skill: { id: 20, slug: 'fmt', name: 'Formatter' },
        created_at: '2026-01-01T00:00:00Z',
      }),
    )
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(url) === '/api/v1/targets/1/assignments') {
        return postCall(url, init)
      }
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      if (String(url) === '/api/v1/targets/1') return mockResponse(mockTargetView)
      if (String(url) === '/api/v1/targets/1/inspect') return mockResponse(mockStatus)
      if (String(url) === '/api/v1/skills') {
        return mockResponse({
          items: [
            { id: 10, slug: 'go-lint', name: 'Go Linter' },
            { id: 20, slug: 'fmt', name: 'Formatter' },
            { id: 21, slug: 'lint', name: 'Linter' },
          ],
          total: 3,
        })
      }
      if (String(url) === '/api/v1/groups') {
        return mockResponse({ items: [{ id: 1, name: 'backend-tools', member_count: 2 }], total: 1 })
      }
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderTargets('/targets/Claude%20User%20Config')
    await screen.findByRole('heading', { name: 'Claude User Config' })
    const picker = screen.getByRole('combobox', { name: 'Single Skill' })
    await user.click(picker)
    await user.click(await screen.findByRole('option', { name: /Formatter/ }))
    await user.click(await screen.findByRole('option', { name: /Linter/ }))
    expect(picker.textContent).toContain('2 selected')
    await user.click(screen.getByRole('button', { name: 'Assign' }))
    await waitFor(() => expect(postCall).toHaveBeenCalledTimes(2))
    expect(JSON.parse((postCall.mock.calls[0][1] as RequestInit).body as string)).toEqual({
      kind: 'skill',
      skill_id: 20,
    })
    expect(JSON.parse((postCall.mock.calls[1][1] as RequestInit).body as string)).toEqual({
      kind: 'skill',
      skill_id: 21,
    })
  })

  it('removing a Group-sourced Skill confirms, then deletes every backing Assignment with Content-Type application/json', async () => {
    const user = userEvent.setup()
    renderTargets('/targets/Claude%20User%20Config')
    await screen.findByRole('heading', { name: 'Claude User Config' })

    const updatedTargetView = {
      ...mockTargetView,
      direct_skills: [],
      groups: [],
      desired_skills: [],
    }

    // Go Linter is backed by a direct Assignment (101) and a Group one (102).
    const deleteCall = vi.fn().mockResolvedValue(mockResponse(updatedTargetView))

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE' && /^\/api\/v1\/targets\/1\/assignments\/\d+$/.test(String(url))) {
        return deleteCall(url, init)
      }
      if (String(url) === '/api/v1/targets') return mockResponse({ items: [mockTargetSummary], total: 1 })
      if (String(url) === '/api/v1/targets/1') return mockResponse(mockTargetView)
      if (String(url) === '/api/v1/targets/1/inspect') return mockResponse(mockStatus)
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      if (String(url) === '/api/v1/groups') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    // A Group-sourced Skill first warns that the whole Group will be removed.
    await user.click(screen.getByRole('button', { name: 'Remove Go Linter' }))
    expect(await screen.findByRole('heading', { name: 'Remove this Skill?' })).toBeTruthy()
    expect(deleteCall).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Remove Group' }))

    await waitFor(() => expect(deleteCall).toHaveBeenCalledTimes(2))
    const deletedIds = deleteCall.mock.calls.map(([u]) => String(u).replace(/.*\//, '')).sort()
    expect(deletedIds).toEqual(['101', '102'])
    const deleteInit = deleteCall.mock.calls[0][1] as RequestInit
    expect(deleteInit.headers).toEqual({ 'Content-Type': 'application/json' })
  })

  it('shows error state for non-existent target name', async () => {
    renderTargets('/targets/nonexistent')
    expect(await screen.findByText('Could not load Target')).toBeTruthy()
  })

  it('marks only detected adapters as Installed in the adapter picker', async () => {
    const user = userEvent.setup()
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
    await user.click(await screen.findByRole('button', { name: 'Add Distribution Target' }))
    await user.click(screen.getByRole('combobox', { name: 'Agent Type' }))

    const options = await screen.findAllByRole('option')
    expect(options.map((o) => o.textContent)).toEqual([
      'Adapter 1Installed',
      'Adapter 2',
      'Adapter 3',
      'Adapter 4',
    ])
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
      if (u === '/api/v1/targets/99/inspect') return mockResponse({ ...mockStatus, target_id: 99, name: '100% coverage' })
      if (u === '/api/v1/skills') return mockResponse({ items: [], total: 0 })
      if (u === '/api/v1/groups') return mockResponse({ items: [], total: 0 })
      return mockResponse({ error: { message: 'not found' } }, false, 404)
    })

    renderTargets('/targets/100%25%20coverage')
    expect(await screen.findByRole('heading', { name: '100% coverage' })).toBeTruthy()
  })
})
