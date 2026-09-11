import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom'
import { SkillDetailPage } from './skill-detail'
import { SkillSyncTab } from './skill-sync-tab'
import { LocaleProvider } from './locale-provider'
import { mockResponse } from './source-fixtures'
import type { Skill } from './skill-api'
import { skillAlpha } from './skill-fixtures'
import {
  conflictSkill,
  defaultHandler,
  failedOutcomeSkill,
  installFetch,
  renderDetail,
  setupMatchMedia,
} from './skill-sync-fixtures'

setupMatchMedia()

function DetailWithBack() {
  const navigate = useNavigate()
  return (
    <div>
      <SkillDetailPage />
      <button onClick={() => navigate(-1)}>go back</button>
    </div>
  )
}

describe('Skill Synchronization Tab', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('deep-links ?tab=synchronization onto the sync tab and back/forward restores it', async () => {
    installFetch(defaultHandler)
    const user = userEvent.setup()
    render(
      <MemoryRouter initialEntries={['/skills/conflict-skill?tab=synchronization']}>
        <Routes>
          <Route path="/skills/:slug" element={<DetailWithBack />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Conflict Skill' })).toBeTruthy()
    const syncTab = screen.getByRole('tab', { name: 'Synchronization' })
    expect(syncTab.getAttribute('aria-selected')).toBe('true')
    expect(await screen.findByText('Latest Sync Action')).toBeTruthy()

    // Switching to Overview pushes a history entry without ?tab.
    await user.click(screen.getByRole('tab', { name: 'Overview' }))
    expect(await screen.findByRole('heading', { name: 'Alpha', level: 2 })).toBeTruthy()
    expect(screen.queryByText('Latest Sync Action')).toBeNull()

    // Browser back returns to the synchronization tab.
    await user.click(screen.getByRole('button', { name: 'go back' }))
    expect(await screen.findByText('Latest Sync Action')).toBeTruthy()
    expect(screen.getByRole('tab', { name: 'Synchronization' }).getAttribute('aria-selected')).toBe('true')
  })

  it('presents the conflict state, digests, and the latest skipped outcome with times', async () => {
    installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    expect(await screen.findAllByText('Sync conflict')).toHaveLength(2)
    expect(screen.getByText(/Synchronization cannot proceed until you Keep Store or Accept Source/)).toBeTruthy()
    expect(screen.getByText('Sync Status')).toBeTruthy()
    expect(screen.getByText('Last checked')).toBeTruthy()
    expect(screen.getByText('Latest Sync Action')).toBeTruthy()
    expect(screen.getByText('Skipped')).toBeTruthy()
    expect(screen.queryByText('Before Digest')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Digests' }))
    expect(screen.getAllByText('sha256:store').length).toBeGreaterThan(0)
    expect(screen.getByText('sha256:base')).toBeTruthy()
    expect(screen.getByText('Before Digest')).toBeTruthy()
    expect(screen.getByText('Source Revision')).toBeTruthy()
    expect(screen.getByText('abc1234')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Sync' })).toBeNull()
  })

  it('presents the latest action error when the outcome failed', async () => {
    installFetch((url, init) => {
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [failedOutcomeSkill], total: 1 })
      if (String(url) === '/api/v1/skills/103') return mockResponse(failedOutcomeSkill)
      return defaultHandler(url, init)
    })
    renderDetail('/skills/conflict-skill?tab=synchronization')

    expect(await screen.findByText('Latest Sync Action')).toBeTruthy()
    expect(screen.getByText('Failed')).toBeTruthy()
    expect(screen.getByText('Latest Error')).toBeTruthy()
    expect(screen.getByText('the Source is not reachable')).toBeTruthy()
  })

  it('shows the stale badge and notice for a stale relationship', async () => {
    const staleSkill: Skill = { ...conflictSkill, sync_stale: true }
    installFetch((url, init) => {
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [staleSkill], total: 1 })
      if (String(url) === '/api/v1/skills/103') return mockResponse(staleSkill)
      return defaultHandler(url, init)
    })
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    expect(screen.getAllByText('stale').length).toBeGreaterThan(0)
    expect(screen.getByText(/observation is stale/)).toBeTruthy()
  })

  it('does not issue any request when an action dialog is opened and cancelled', async () => {
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    await user.click(screen.getByRole('button', { name: 'Accept Source' }))

    expect(await screen.findByText('Accept Source content?')).toBeTruthy()
    expect(fetchMock.mock.calls.filter(([u]) => String(u).includes('accept-source'))).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(screen.queryByText('Accept Source content?')).toBeNull()
    })
    expect(fetchMock.mock.calls.filter(([u]) => String(u).includes('accept-source'))).toHaveLength(0)
  })

  it('Accept Source requires its own explicit dialog, never a generic confirm', async () => {
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    await user.click(screen.getByRole('button', { name: 'Accept Source' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Accept Source content?')).toBeTruthy()
    expect(within(dialog).getByText(/snapshotted and replaced/)).toBeTruthy()

    await user.click(within(dialog).getByRole('button', { name: 'Accept Source' }))

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(([u]) => String(u) === '/api/v1/skills/103/accept-source')
      expect(post).toBeTruthy()
      expect(post![1]).toMatchObject({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
    })
    expect(await screen.findByText('Accepted Source')).toBeTruthy()
  })

  it('Keep Store and Rollback use their own named dialogs', async () => {
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')

    await user.click(screen.getByRole('button', { name: 'Keep Store' }))
    expect(await screen.findByText('Keep Store content?')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Keep Store' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u) === '/api/v1/skills/103/keep-store')).toBe(true)
    })
    expect(await screen.findByText('Kept Store')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Rollback' }))
    expect(await screen.findByText('Roll back to the previous snapshot?')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Rollback' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u) === '/api/v1/skills/103/rollback')).toBe(true)
    })
    expect(await screen.findByText('Rolled back')).toBeTruthy()
  })

  it('runs the safe check without offering Sync during conflict', async () => {
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    renderDetail('/skills/conflict-skill?tab=synchronization')

    await screen.findByText('Three-Way Diff')
    expect(screen.queryByRole('button', { name: 'Sync' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Check' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u) === '/api/v1/skills/103/check')).toBe(true)
    })
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})

describe('Skill Synchronization Locale', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  afterEach(() => cleanup())

  it('renders the sync tab in Simplified Chinese with the stored locale', async () => {
    localStorage.setItem('locale', 'zh-CN')
    installFetch(defaultHandler)
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/conflict-skill?tab=synchronization']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )

    expect(await screen.findByText('同步状态')).toBeTruthy()
    expect(screen.getAllByText('同步冲突').length).toBeGreaterThan(0)
    expect(screen.getByText('最近同步操作')).toBeTruthy()
    expect(screen.getByText('三向差异')).toBeTruthy()
    expect(screen.getByRole('button', { name: '接受来源' })).toBeTruthy()
  })

  it('localizes the unbound notice in English', async () => {
    const betaSkill: Skill = {
      ...skillAlpha,
      id: 104,
      slug: 'beta-skill',
      name: 'Beta Skill',
      description: '',
      sync_status: 'unbound',
      binding: undefined,
    }
    installFetch((url, init) => {
      if (String(url) === '/api/v1/skills') return mockResponse({ items: [skillAlpha, betaSkill], total: 2 })
      if (String(url) === '/api/v1/skills/104') return mockResponse(betaSkill)
      return defaultHandler(url, init)
    })
    render(
      <MemoryRouter initialEntries={['/skills/beta-skill?tab=synchronization']}>
        <Routes>
          <Route path="/skills/:slug" element={<SkillDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByText('Unbound Skill')).toBeTruthy()
    expect(screen.getByText(/This Skill is not bound to a Source/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Check' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Sync' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Accept Source' })).toBeNull()
  })
})

describe('SkillSyncTab standalone', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('refreshes the skill through check and re-renders the new checked time', async () => {
    const onSkillUpdated = vi.fn()
    const fetchMock = installFetch(defaultHandler)
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <SkillSyncTab skill={conflictSkill} onSkillUpdated={onSkillUpdated} />
      </MemoryRouter>,
    )

    await screen.findByText('Three-Way Diff')
    await user.click(screen.getByRole('button', { name: 'Check' }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([u]) => String(u) === '/api/v1/skills/103/check')).toBe(true)
      expect(onSkillUpdated).toHaveBeenCalled()
    })
  })
})
