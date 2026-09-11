import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { SkillContentTab } from './skill-content-tab'
import { setupMatchMedia, mockResponse } from './source-fixtures'
import { skillAlpha, skillAlphaContent } from './skill-fixtures'

setupMatchMedia()

function installContentFetch(handler: (url: string) => Response): void {
  window.fetch = vi.fn().mockImplementation(async (url: RequestInfo | URL) => handler(String(url)))
}

describe('SkillContentTab', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => cleanup())

  it('renders frontmatter keys and values in the returned order', async () => {
    installContentFetch(() => mockResponse(skillAlphaContent))
    render(<SkillContentTab skill={skillAlpha} />)

    expect(await screen.findByRole('heading', { name: 'Frontmatter' })).toBeTruthy()
    const terms = screen.getAllByRole('term').map((el) => el.textContent)
    const definitions = screen.getAllByRole('definition').map((el) => el.textContent)
    expect(terms).toEqual(['name', 'description', 'allowed-tools'])
    expect(definitions).toEqual(['alpha', 'First test skill in local store', 'Bash, Read'])
  })

  it('renders the Markdown body as elements, not raw source', async () => {
    installContentFetch(() => mockResponse(skillAlphaContent))
    render(<SkillContentTab skill={skillAlpha} />)

    // The body's `#` heading shifts one level down: the page h1 stays the Skill name's.
    expect(await screen.findByRole('heading', { name: 'Alpha', level: 2 })).toBeTruthy()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('npm test')).toBeTruthy()
    expect(screen.queryByText(/# Alpha/)).toBeNull()
    expect(screen.queryByText(/```/)).toBeNull()
  })

  it('surfaces the error Alert when the content endpoint responds 404', async () => {
    installContentFetch(
      () =>
        new Response(JSON.stringify({ error: { message: 'skill content not found' } }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        }),
    )
    render(<SkillContentTab skill={skillAlpha} />)

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Could not load Skill content')
    expect(alert.textContent).toContain('skill content not found')
  })

  it('shows the empty-body line when SKILL.md has no body', async () => {
    installContentFetch(() => mockResponse({ ...skillAlphaContent, body: '  \n' }))
    render(<SkillContentTab skill={skillAlpha} />)

    expect(await screen.findByText('The SKILL.md body is empty.')).toBeTruthy()
  })
})
