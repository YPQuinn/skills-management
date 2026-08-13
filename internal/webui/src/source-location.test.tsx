import { describe, it, expect, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import { SourceLocationIcon } from './source-location'
import { detail } from './source-fixtures'
import type { SourceDetail } from './source-api'

function iconFor(source: SourceDetail): Element | null {
  const { container } = render(<SourceLocationIcon source={source} />)
  return container.querySelector('svg')
}

function providerOf(source: SourceDetail): string | null | undefined {
  return iconFor(source)?.getAttribute('data-source-provider')
}

function gitIconAt(location: string): string | null | undefined {
  return providerOf({ ...detail, kind: 'git', location })
}

describe('SourceLocationIcon', () => {
  afterEach(() => cleanup())

  it('marks local sources with the desktop icon', () => {
    const svg = iconFor(detail)
    expect(svg?.getAttribute('aria-hidden')).toBe('true')
    expect(svg?.getAttribute('data-source-provider')).toBe('local')
  })

  it('marks GitHub locations with the GitHub brand icon', () => {
    const svg = iconFor({ ...detail, kind: 'git', location: 'https://github.com/org/repo' })
    expect(svg?.getAttribute('aria-hidden')).toBe('true')
    expect(svg?.getAttribute('data-source-provider')).toBe('github')
  })

  it('marks GitLab locations with the GitLab brand icon', () => {
    const svg = iconFor({ ...detail, kind: 'git', location: 'git@gitlab.com:group/project.git' })
    expect(svg?.getAttribute('data-source-provider')).toBe('gitlab')
  })

  it('marks other git locations with the generic Git icon', () => {
    const svg = iconFor({ ...detail, kind: 'git', location: 'git@git.example.com:org/repo.git' })
    expect(svg?.getAttribute('data-source-provider')).toBe('git')
  })

  it('recognises GitHub and GitLab hosts case-insensitively across URL and scp-like forms', () => {
    const cases: Array<{ location: string; provider: string }> = [
      { location: 'HTTPS://GITHUB.COM/org/repo', provider: 'github' },
      { location: 'ssh://git@GitHub.com/org/repo.git', provider: 'github' },
      { location: 'git@GITLAB.COM:group/project.git', provider: 'gitlab' },
      { location: 'http://GitLab.com/group/project', provider: 'gitlab' },
    ]
    for (const { location, provider } of cases) {
      cleanup()
      expect(gitIconAt(location)).toBe(provider)
    }
  })

  it('does not mislabel look-alike hosts or paths that only mention a host name', () => {
    const cases = [
      'https://github.com.evil/org/repo',
      'https://gitlab.com.evil/group/project',
      'https://example.com/gitlab/repo',
      'git@gitlab.example.com:group/project.git',
    ]
    for (const location of cases) {
      cleanup()
      expect(gitIconAt(location)).toBe('git')
    }
  })

  it('falls back to git for unparseable locations', () => {
    cleanup()
    expect(gitIconAt('https://')).toBe('git')
    cleanup()
    expect(gitIconAt('just-a-name')).toBe('git')
  })
})
