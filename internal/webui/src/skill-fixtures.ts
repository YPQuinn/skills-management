import type { Skill, ImportResponse } from './skill-api'

export const skillAlpha: Skill = {
  id: 101,
  slug: 'alpha',
  name: 'Alpha Skill',
  description: 'First test skill in local store',
  store_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
  baseline_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
  created_at: '2026-08-11T10:00:00Z',
  updated_at: '2026-08-11T10:00:00Z',
  binding: {
    source_id: 1,
    source_name: 'local-one',
    relative_dir: 'skills/alpha',
    digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
    source_commit: 'abc1234',
    imported_at: '2026-08-11T10:00:00Z',
  },
}

export const skillBeta: Skill = {
  id: 102,
  slug: 'beta',
  name: 'Beta Skill',
  description: 'Second test skill without binding',
  store_digest: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
  baseline_digest: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
  created_at: '2026-08-11T11:00:00Z',
  updated_at: '2026-08-11T11:00:00Z',
}

export const mockImportSuccess: ImportResponse = {
  items: [
    {
      status: 'imported',
      relative_dir: 'skills/alpha',
      requested_slug: 'alpha',
      slug: 'alpha',
      skill_id: 101,
    },
  ],
  summary: {
    imported: 1,
    skipped: 0,
    failed: 0,
  },
}

export const mockImportConflict: ImportResponse = {
  items: [
    {
      status: 'skipped_conflict',
      relative_dir: 'skills/alpha',
      requested_slug: 'alpha',
      replaces: {
        skill_id: 101,
        slug: 'alpha',
        name: 'Alpha Skill',
      },
      code: 'slug_conflict',
      message: 'Skill slug "alpha" already exists',
    },
  ],
  summary: {
    imported: 0,
    skipped: 1,
    failed: 0,
  },
}
