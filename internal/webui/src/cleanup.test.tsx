import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SkillDetailPage } from './skill-detail'
import { TargetDeleteAction } from './target-delete-action'
import { setupMatchMedia, mockResponse, detail } from './source-fixtures'
import { skillAlpha } from './skill-fixtures'
import { LocaleProvider } from './locale-provider'

setupMatchMedia()

function isDisabled(el: HTMLElement | undefined | null): boolean {
  if (!el) return false
  return (
    el.hasAttribute('disabled') ||
    el.getAttribute('aria-disabled') === 'true' ||
    el.hasAttribute('data-disabled')
  )
}

const skillDeletion = {
  skill: { id: 101, slug: 'alpha', name: 'Alpha Skill' },
  groups: [{ id: 1, name: 'eng' }],
  targets: [{ id: 2, name: 'mine', direct: true, groups: [] }],
  links: [{ target_id: 2, target_name: 'mine', skill_id: 101, slug: 'alpha' }],
  referenced: true,
}

describe('Skill cleanup dialogs', () => {
  let previewGate: Promise<void>
  let releasePreview: () => void

  beforeEach(() => {
    vi.restoreAllMocks()
    previewGate = new Promise<void>((resolve) => {
      releasePreview = resolve
    })
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (urlStr === '/api/v1/skills') {
        return mockResponse({ items: [skillAlpha], total: 1 })
      }
      if (urlStr === '/api/v1/skills/101/deletion') {
        await previewGate
        return mockResponse(skillDeletion)
      }
      if (urlStr === '/api/v1/skills/101' && init?.method === 'DELETE') {
        return mockResponse({
          skill: { id: 101, slug: 'alpha', name: 'Alpha Skill' },
          removed_assignments: 1,
          removed_memberships: 1,
          links: [{ target_id: 2, target_name: 'mine', skill_id: 101, slug: 'alpha', result: 'removed' }],
        })
      }
      if (urlStr === '/api/v1/skills/101') {
        return mockResponse(skillAlpha)
      }
      if (urlStr === '/api/v1/skills/101/detach' && init?.method === 'POST') {
        return mockResponse({ ...skillAlpha, binding: undefined, sync_status: 'unbound' })
      }
      if (urlStr === '/api/v1/sources') {
        return mockResponse({ items: [{ id: 1, name: 'local-one', kind: 'local', location: '/tmp' }], total: 1 })
      }
      if (urlStr === '/api/v1/sources/1') {
        return mockResponse(detail)
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
  })

  afterEach(() => cleanup())

  it('confirms Skill deletion after showing the referenced preview', async () => {
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/alpha']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
            <Route path="/skills" element={<div>Skills list</div>} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )

    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByRole('heading', { name: 'Delete Skill Alpha Skill?' })).toBeTruthy()
    expect(screen.getByText('Loading preview…')).toBeTruthy()
    const confirmWhileLoading = screen.getAllByRole('button', { name: 'Delete' }).at(-1)
    expect(isDisabled(confirmWhileLoading)).toBe(true)
    await user.click(confirmWhileLoading!)
    expect(
      (window.fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls.some(
        (c) => String(c[0]).includes('/skills/101') && (c[1] as RequestInit | undefined)?.method === 'DELETE',
      ),
    ).toBe(false)
    releasePreview()
    expect(await screen.findByText(/This Skill is referenced/)).toBeTruthy()
    expect(screen.getByText(/Groups: eng/)).toBeTruthy()

    const confirms = screen.getAllByRole('button', { name: 'Delete' })
    await user.click(confirms[confirms.length - 1])
    await waitFor(() => {
      expect(screen.getByText('Skills list')).toBeTruthy()
    })
    const calls = (window.fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls
    const deleted = calls.some((c) => String(c[0]).includes('/skills/101') && (c[1] as RequestInit | undefined)?.method === 'DELETE')
    expect(deleted).toBe(true)
  })

  it('detaches a bound Skill after confirmation', async () => {
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/alpha']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )

    releasePreview()
    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Detach' }))
    expect(await screen.findByRole('heading', { name: 'Detach Skill Alpha Skill?' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Detach' }))
    await waitFor(() => {
      const calls = (window.fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls
      expect(calls.some((c) => String(c[0]).includes('/detach'))).toBe(true)
    })
  })

  it('shows the Source name in the rebind trigger after selection', async () => {
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/alpha']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )

    releasePreview()
    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Rebind' }))
    expect(await screen.findByRole('heading', { name: 'Rebind Skill Alpha Skill' })).toBeTruthy()

    const sourceSelect = screen.getByRole('combobox', { name: 'Source' })
    await user.click(sourceSelect)
    await user.click(await screen.findByRole('option', { name: 'local-one' }))

    expect(sourceSelect.textContent).toContain('local-one')
    expect(sourceSelect.textContent).not.toMatch(/^\s*1\s*$/)
  })

  it('does not allow confirm when the preview fails to load', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      const urlStr = String(url)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [skillAlpha], total: 1 })
      if (urlStr === '/api/v1/skills/101') return mockResponse(skillAlpha)
      if (urlStr === '/api/v1/skills/101/deletion') {
        return mockResponse({ error: { message: 'preview exploded' } }, false, 500)
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/alpha']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )
    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('preview exploded')).toBeTruthy()
    const confirm = screen.getAllByRole('button', { name: 'Delete' }).at(-1)
    expect(isDisabled(confirm)).toBe(true)
  })

  it('shows an ownership-lost warning instead of leaving the page', async () => {
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) => {
      const urlStr = String(url)
      if (urlStr === '/api/v1/skills') return mockResponse({ items: [skillAlpha], total: 1 })
      if (urlStr === '/api/v1/skills/101/deletion') return mockResponse(skillDeletion)
      if (urlStr === '/api/v1/skills/101' && init?.method === 'DELETE') {
        return mockResponse({
          skill: { id: 101, slug: 'alpha', name: 'Alpha Skill' },
          removed_assignments: 1,
          removed_memberships: 1,
          links: [{ target_id: 2, target_name: 'mine', skill_id: 101, slug: 'alpha', result: 'ownership_lost' }],
        })
      }
      if (urlStr === '/api/v1/skills/101') return mockResponse(skillAlpha)
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter initialEntries={['/skills/alpha']}>
          <Routes>
            <Route path="/skills/:slug" element={<SkillDetailPage />} />
            <Route path="/skills" element={<div>Skills list</div>} />
          </Routes>
        </MemoryRouter>
      </LocaleProvider>,
    )
    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText(/This Skill is referenced/)).toBeTruthy()
    const confirms = screen.getAllByRole('button', { name: 'Delete' })
    await user.click(confirms[confirms.length - 1])
    expect(await screen.findByText(/Ownership was lost for: alpha/)).toBeTruthy()
    expect(screen.queryByText('Skills list')).toBeNull()
  })
})

describe('Target delete preview', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('shows Assignments and keeps confirm disabled until the preview loads', async () => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => {
      if (String(url) === '/api/v1/targets/2/deletion') {
        await gate
        return mockResponse({
          target: { id: 2, name: 'mine' },
          assignments: [{ id: 9, kind: 'skill', skill: { id: 101, slug: 'alpha', name: 'Alpha Skill' } }],
          links: [{ target_id: 2, target_name: 'mine', skill_id: 101, slug: 'alpha' }],
        })
      }
      return mockResponse({ error: { message: 'Not found' } }, false, 404)
    })
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <MemoryRouter>
          <TargetDeleteAction targetId={2} name="mine" />
        </MemoryRouter>
      </LocaleProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Loading preview…')).toBeTruthy()
    expect(isDisabled(screen.getAllByRole('button', { name: 'Delete' }).at(-1))).toBe(true)
    release()
    expect(await screen.findByText(/Assignments: Alpha Skill/)).toBeTruthy()
    expect(screen.getByText(/Managed Links that will be removed: alpha/)).toBeTruthy()
    expect(isDisabled(screen.getAllByRole('button', { name: 'Delete' }).at(-1))).toBe(false)
  })
})
