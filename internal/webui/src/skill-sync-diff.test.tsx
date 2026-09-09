import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { mockResponse } from './source-fixtures'
import {
  defaultHandler,
  diffResult,
  installFetch,
  renderDetail,
  setupMatchMedia,
} from './skill-sync-fixtures'

setupMatchMedia()

describe('Skill Synchronization Diff', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('renders the three comparisons with change badges, unified text, and binary metadata', async () => {
    installFetch(defaultHandler)
    renderDetail('/skills/conflict-skill?tab=synchronization')

    expect(await screen.findByText('Three-Way Diff')).toBeTruthy()
    expect(await screen.findByText('notes.md')).toBeTruthy()
    expect(screen.getByText('logo.png')).toBeTruthy()
    expect(screen.getByText('run.sh')).toBeTruthy()
    expect(screen.getByText('old.md')).toBeTruthy()
    expect(screen.getByText('kind-change')).toBeTruthy()
    expect(screen.queryByRole('heading', { name: /Baseline.*Source/i })).toBeNull()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /notes\.md/ }))
    expect(screen.getByText('added')).toBeTruthy()
    expect(screen.getByText('+upstream', { exact: false })).toBeTruthy()
    expect(screen.queryByText('deleted')).toBeNull()

    await user.click(screen.getByRole('button', { name: /logo\.png/ }))
    expect(screen.getByText('content changed')).toBeTruthy()
    expect(screen.getByText('300000 bytes', { exact: false })).toBeTruthy()
    expect(screen.getByText('301000 bytes', { exact: false })).toBeTruthy()
    expect(screen.getByText('bb22')).toBeTruthy()
    expect(screen.getByText('cc33')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /run\.sh/ }))
    expect(screen.getByText('executable bit')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /kind-change/ }))
    expect(screen.getByText('Source vs Store')).toBeTruthy()
    expect(screen.getByText('node type')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /old\.md/ }))
    expect(screen.getByText('deleted')).toBeTruthy()
  })

  it('requests the diff again with a path filter', async () => {
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    const filter = await screen.findByLabelText('Filter diff by path')
    await user.type(filter, 'SKILL.md')

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u) === '/api/v1/skills/103/diff?path=SKILL.md')).toBe(true)
    })
  })

  it('shows a single empty state when every comparison has no differences', async () => {
    installFetch((url, init) => {
      if (String(url) === '/api/v1/skills/103/diff') {
        return mockResponse({
          ...diffResult,
          comparisons: diffResult.comparisons.map((c) => ({ ...c, entries: [] })),
        })
      }
      return defaultHandler(url, init)
    })
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    expect(await screen.findByText('No differences.')).toBeTruthy()
    expect(screen.queryByText('No differences between these two sides.')).toBeNull()
  })

  it('surfaces diff failures with the server message', async () => {
    installFetch((url) => {
      if (String(url) === '/api/v1/skills/103/diff') {
        return new Response(JSON.stringify({ error: { message: 'the Source is not reachable' } }), {
          status: 409,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultHandler(url)
    })
    renderDetail('/skills/conflict-skill?tab=synchronization')

    expect(await screen.findByText('Could not load the diff')).toBeTruthy()
    expect(screen.getByText('the Source is not reachable')).toBeTruthy()
  })
})
