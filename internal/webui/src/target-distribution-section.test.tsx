import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TargetDistributionSection } from './target-distribution-section'
import { LocaleProvider } from './locale-provider'
import type { DistributionStatus } from './distribution-api'

import { setupMatchMedia } from './source-fixtures'

setupMatchMedia()

const statusLinked: DistributionStatus = {
  target_id: 1,
  name: 'mine',
  path: '/tmp/skills',
  state: 'ok',
  inspected_at: '2026-01-01T00:00:00Z',
  stale: false,
  items: [
    {
      skill_id: 10,
      slug: 'demo',
      desired: 'present',
      observed: 'linked',
      managed: true,
      adoptable: false,
      stale: false,
      last_result: 'created',
      expected_path: '/tmp/skills/demo',
    },
  ],
  last_result: 'succeeded',
}

const statusAdoptable: DistributionStatus = {
  target_id: 1,
  name: 'mine',
  path: '/tmp/skills',
  state: 'ok',
  inspected_at: '2026-01-01T00:00:00Z',
  stale: false,
  items: [
    {
      skill_id: 10,
      slug: 'demo',
      desired: 'present',
      observed: 'conflict',
      managed: false,
      adoptable: true,
      node_kind: 'symlink',
      stale: false,
      last_result: 'blocked_conflict',
    },
  ],
  last_result: 'blocked',
}

function renderSection(initial: DistributionStatus) {
  const onChanged = vi.fn()
  render(
    <LocaleProvider>
      <TargetDistributionSection targetId={1} initial={initial} onChanged={onChanged} />
    </LocaleProvider>,
  )
  return { onChanged }
}

describe('TargetDistributionSection', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        const method = init?.method ?? 'GET'
        if (url.includes('/inspect')) {
          return new Response(JSON.stringify(statusLinked), { status: 200 })
        }
        if (url.includes('/distribute') && JSON.parse(String(init?.body)).dry_run === true) {
          return new Response(
            JSON.stringify({
              target_id: 1,
              dry_run: true,
              outcome: 'succeeded',
              stale: false,
              items: [{ skill_id: 10, slug: 'demo', desired: 'present', observed: 'missing', action: 'created', result: 'created' }],
              summary: { total: 1, no_op: 0, created: 1, removed: 0, adopted: 0, blocked_conflict: 0, blocked_broken: 0, ownership_lost: 0, failed: 0 },
            }),
            { status: 200 },
          )
        }
        if (url.includes('/distribute') && method === 'POST') {
          return new Response(
            JSON.stringify({
              target_id: 1,
              dry_run: false,
              outcome: 'succeeded',
              stale: false,
              items: [{ skill_id: 10, slug: 'demo', desired: 'present', observed: 'linked', action: 'no_op', result: 'no_op' }],
              summary: { total: 1, no_op: 1, created: 0, removed: 0, adopted: 0, blocked_conflict: 0, blocked_broken: 0, ownership_lost: 0, failed: 0 },
            }),
            { status: 200 },
          )
        }
        if (url.includes('/adopt')) {
          return new Response(
            JSON.stringify({ target_id: 1, skill_id: 10, slug: 'demo', link_path: '/tmp/skills/demo', raw_target: '/store/demo', adopted_at: '2026-01-01T00:00:00Z', result: 'adopted' }),
            { status: 200 },
          )
        }
        return new Response(JSON.stringify({}), { status: 404 })
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the stored status and refreshes on Inspect', async () => {
    const { onChanged } = renderSection(statusLinked)
    const user = userEvent.setup()
    expect(screen.getByText('demo')).toBeTruthy()
    expect(screen.getByText('Linked')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Inspect' }))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  it('renders a five-column status table with a last-result cell', () => {
    renderSection(statusLinked)
    const table = screen.getByRole('table')
    expect(within(table).getAllByRole('columnheader')).toHaveLength(5)
    expect(within(table).getAllByRole('cell')).toHaveLength(5)
    expect(within(table).getByText('Created')).toBeTruthy()
    expect(table.querySelector('ul')).toBeNull()
  })

  it('previews the dry-run plan without distributing', async () => {
    renderSection(statusLinked)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() => expect(screen.getByText('Distribution plan')).toBeTruthy())
    expect(screen.getAllByText(/demo: Created/).length).toBeGreaterThan(0)
    const fetchMock = vi.mocked(fetch)
    const calls = fetchMock.mock.calls.map((c) => String(c[0]))
    expect(calls.some((u) => u.includes('/distribute') && JSON.parse(String(fetchMock.mock.calls[calls.indexOf(u)][1]?.body)).dry_run === true)).toBe(true)
  })

  it('confirms and runs distribution, then shows the result', async () => {
    renderSection(statusLinked)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Distribute' }))
    await waitFor(() => expect(screen.getByText('Distribute to this Target?')).toBeTruthy())
    const confirmButtons = screen.getAllByRole('button', { name: 'Distribute' })
    await user.click(confirmButtons[confirmButtons.length - 1])
    await waitFor(() => expect(screen.getByText('Distribution result')).toBeTruthy())
    expect(screen.getByText(/demo: No change/)).toBeTruthy()
  })

  it('resets status and dialogs when switching Targets', async () => {
    const other: DistributionStatus = {
      ...statusLinked,
      target_id: 2,
      name: 'other',
      items: [
        {
          skill_id: 20,
          slug: 'other',
          desired: 'present',
          observed: 'missing',
          managed: false,
          adoptable: false,
          stale: false,
          last_result: 'removed',
        },
      ],
    }
    const onChanged = vi.fn()
    const { rerender } = render(
      <LocaleProvider>
        <TargetDistributionSection targetId={1} initial={statusLinked} onChanged={onChanged} />
      </LocaleProvider>,
    )
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() => expect(screen.getByText('Distribution plan')).toBeTruthy())
    expect(screen.getByText('demo')).toBeTruthy()

    rerender(
      <LocaleProvider>
        <TargetDistributionSection targetId={2} initial={other} onChanged={onChanged} />
      </LocaleProvider>,
    )
    expect(screen.queryByText('Distribution plan')).toBeNull()
    expect(screen.queryByText('demo')).toBeNull()
    expect(screen.getByText('other')).toBeTruthy()
  })

  it('adopts an eligible conflict symlink after confirmation', async () => {
    renderSection(statusAdoptable)
    const user = userEvent.setup()
    expect(screen.getByText('Conflict')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Adopt' }))
    await waitFor(() => expect(screen.getByText('Adopt this link?')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: /^Adopt$/ }))
    await waitFor(() => {
      const fetchMock = vi.mocked(fetch)
      expect(fetchMock.mock.calls.some((c) => String(c[0]).includes('/adopt'))).toBe(true)
    })
  })
})
