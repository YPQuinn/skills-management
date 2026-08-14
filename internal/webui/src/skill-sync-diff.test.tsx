import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  defaultHandler,
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
    // The comparison headings render once the diff fetch resolves; wait for
    // the first one instead of racing the in-flight request.
    expect(await screen.findByRole('heading', { name: /Baseline.*Source/i })).toBeTruthy()
    expect(screen.getByRole('heading', { name: /Baseline.*Store/i })).toBeTruthy()
    expect(screen.getByRole('heading', { name: /Source.*Store/i })).toBeTruthy()

    // add with unified text
    expect(screen.getByText('notes.md')).toBeTruthy()
    expect(screen.getByText('added')).toBeTruthy()
    expect(screen.getByText('+upstream', { exact: false })).toBeTruthy()

    // binary content change: sizes and digests instead of text
    expect(screen.getByText('logo.png')).toBeTruthy()
    expect(screen.getByText('content changed')).toBeTruthy()
    expect(screen.getByText('300000 bytes', { exact: false })).toBeTruthy()
    expect(screen.getByText('301000 bytes', { exact: false })).toBeTruthy()
    expect(screen.getByText('bb22')).toBeTruthy()
    expect(screen.getByText('cc33')).toBeTruthy()

    // exec bit and node type
    expect(screen.getByText('run.sh')).toBeTruthy()
    expect(screen.getByText('executable bit')).toBeTruthy()
    expect(screen.getByText('kind-change')).toBeTruthy()
    expect(screen.getByText('node type')).toBeTruthy()

    // delete on the baseline→source side
    expect(screen.getByText('old.md')).toBeTruthy()
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

  it('shows the empty state for comparisons without differences', async () => {
    installFetch(defaultHandler)
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    expect(await screen.findByText('No differences between these two sides.')).toBeTruthy()
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
