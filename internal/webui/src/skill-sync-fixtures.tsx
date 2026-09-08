// Shared fixtures and fetch handlers for the Skill synchronization test
// files: one conflict Skill, one three-way diff response, and the default
// REST mock wiring every sync route.
import { vi } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { SkillDetailPage } from './skill-detail'
import { mockResponse } from './source-fixtures'
import type { Skill } from './skill-api'
import type { DiffResult } from './sync-api'
import { skillAlpha } from './skill-fixtures'

export function setupMatchMedia(): void {
  window.matchMedia =
    window.matchMedia ||
    function () {
      return {
        matches: false,
        addListener: function () {},
        removeListener: function () {},
        addEventListener: function () {},
        removeEventListener: function () {},
        dispatchEvent: function () {},
      }
    }
}

export const conflictSkill: Skill = {
  ...skillAlpha,
  id: 103,
  slug: 'conflict-skill',
  name: 'Conflict Skill',
  description: 'Skill whose Source and Store both changed',
  store_digest: 'sha256:store',
  baseline_digest: 'sha256:base',
  sync_status: 'conflict',
  sync_stale: false,
  sync_checked_at: '2026-08-11T11:00:00Z',
  has_previous_snapshot: true,
  binding: {
    source_id: 7,
    source_name: 'upstream',
    relative_dir: 'skills/conflict',
    digest: 'sha256:src',
    source_commit: 'abc1234',
    imported_at: '2026-08-11T10:00:00Z',
  },
  last_sync: {
    action: 'sync',
    result: 'skipped',
    started_at: '2026-08-11T11:00:00Z',
    completed_at: '2026-08-11T11:00:01Z',
    before_digest: 'sha256:store',
    revision: 'abc1234',
  },
}

// conflictSkill with a failed latest outcome, for the error-presentation
// behavior test.
export const failedOutcomeSkill: Skill = {
  ...conflictSkill,
  last_sync: {
    action: 'sync',
    result: 'failed',
    started_at: '2026-08-11T12:00:00Z',
    completed_at: '2026-08-11T12:00:02Z',
    before_digest: 'sha256:store',
    after_digest: 'sha256:store',
    revision: 'abc1234',
    error: 'the Source is not reachable',
  },
}

export const diffResult: DiffResult = {
  skill_id: 103,
  slug: 'conflict-skill',
  source_digest: 'sha256:src',
  store_digest: 'sha256:store',
  baseline_digest: 'sha256:base',
  comparisons: [
    {
      from: 'baseline',
      to: 'source',
      entries: [
        {
          path: 'notes.md',
          changes: ['add'],
          to: { kind: 'file', size: 9, digest: 'aa11' },
          text: { unified: '--- /dev/null\n+++ b/notes.md\n@@ -0,0 +1 @@\n+upstream\n' },
        },
        {
          path: 'logo.png',
          changes: ['content'],
          from: { kind: 'file', size: 300000, digest: 'bb22' },
          to: { kind: 'file', size: 301000, digest: 'cc33' },
        },
        {
          path: 'run.sh',
          changes: ['exec'],
          from: { kind: 'file', size: 12, digest: 'dd44' },
          to: { kind: 'file', exec: true, size: 12, digest: 'dd44' },
        },
        {
          path: 'old.md',
          changes: ['delete'],
          from: { kind: 'file', size: 4, digest: 'ee55' },
        },
      ],
    },
    { from: 'baseline', to: 'store', entries: [] },
    {
      from: 'source',
      to: 'store',
      entries: [
        { path: 'kind-change', changes: ['node_type'], from: { kind: 'file', size: 2, digest: 'ff66' }, to: { kind: 'dir' } },
        { path: 'notes.md', changes: ['delete'], from: { kind: 'file', size: 9, digest: 'aa11' } },
      ],
    },
  ],
}

export function installFetch(
  handler: (url: string, init?: RequestInit) => Response | Promise<Response>,
): ReturnType<typeof vi.fn> {
  const mock = vi.fn().mockImplementation(async (url: RequestInfo | URL, init?: RequestInit) =>
    handler(String(url), init),
  )
  window.fetch = mock as unknown as typeof fetch
  return mock
}

export function defaultHandler(url: string, init?: RequestInit): Response {
  const urlStr = String(url)
  if (urlStr === '/api/v1/skills') return mockResponse({ items: [conflictSkill], total: 1 })
  if (urlStr === '/api/v1/skills/103') return mockResponse(conflictSkill)
  if (urlStr === '/api/v1/skills/103/diff') return mockResponse(diffResult)
  if (urlStr === '/api/v1/skills/103/diff?path=SKILL.md') {
    return mockResponse({
      ...diffResult,
      comparisons: diffResult.comparisons.map((c) => ({ ...c, entries: [] })),
    })
  }
  if (init?.method === 'POST' && urlStr === '/api/v1/skills/103/check') {
    return mockResponse({ ...conflictSkill, sync_checked_at: '2026-08-11T12:00:00Z' })
  }
  if (init?.method === 'POST' && urlStr === '/api/v1/skills/103/sync') {
    return mockResponse({ skill_id: 103, slug: 'conflict-skill', status: 'conflict', stale: false, action: 'sync', result: 'skipped' })
  }
  if (init?.method === 'POST' && urlStr === '/api/v1/skills/103/keep-store') {
    return mockResponse({ skill_id: 103, slug: 'conflict-skill', status: 'store_changed', stale: false, action: 'keep_store', result: 'kept_store' })
  }
  if (init?.method === 'POST' && urlStr === '/api/v1/skills/103/accept-source') {
    return mockResponse({ skill_id: 103, slug: 'conflict-skill', status: 'in_sync', stale: false, action: 'accept_source', result: 'accepted_source' })
  }
  if (init?.method === 'POST' && urlStr === '/api/v1/skills/103/rollback') {
    return mockResponse({ skill_id: 103, slug: 'conflict-skill', status: 'store_changed', stale: false, action: 'rollback', result: 'rolled_back' })
  }
  return mockResponse({ error: { message: 'Not found' } }, false, 404)
}

export function renderDetail(initialEntry = '/skills/conflict-skill') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/skills/:slug" element={<SkillDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}
