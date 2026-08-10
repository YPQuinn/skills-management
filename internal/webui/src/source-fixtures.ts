import type { SourceDetail, SourceSummary } from './source-api'

// Shared fixtures for the Source test files. Appica's theme and select
// primitives query matchMedia in jsdom; one stub keeps every test
// deterministic.
export function setupMatchMedia(): void {
  window.matchMedia = window.matchMedia || function() {
    return {
      matches: false,
      addListener: function() {},
      removeListener: function() {},
      addEventListener: function() {},
      removeEventListener: function() {},
      dispatchEvent: function() {},
    }
  }
}

export const summary: SourceSummary = {
  id: 1,
  name: 'local-one',
  kind: 'local',
  location: '/tmp/skills',
  available: true,
  stale: false,
  entry_count: 2,
  last_checked_at: '2026-08-10T12:00:00Z',
}

export const detail: SourceDetail = {
  ...summary,
  created_at: '2026-08-10T11:00:00Z',
  updated_at: '2026-08-10T12:00:00Z',
  inventory: [
    { relative_dir: 'skills/alpha', name: 'Alpha', description: 'first skill' },
    { relative_dir: 'skills/beta', name: 'Beta', description: 'second skill' },
  ],
  issues: [],
}

export const unavailableDetail: SourceDetail = {
  ...detail,
  available: false,
  stale: true,
  last_error: 'checking /tmp/skills: no such directory',
  inventory: detail.inventory,
}

export function mockResponse(body: unknown, ok = true, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status: ok ? status : 400,
    headers: { 'Content-Type': 'application/json' },
  })
}
