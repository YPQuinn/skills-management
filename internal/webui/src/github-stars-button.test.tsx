import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { GitHubStarsButton } from './github-stars-button'

const repository = 'YPQuinn/skills-management'

describe('GitHubStarsButton', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('links to the repository and displays its live star count', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ stargazers_count: 42 }),
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <GitHubStarsButton
        repo={repository}
        accessibleLabel="View and star skills-management on GitHub"
      />,
    )

    const link = screen.getByRole('link', {
      name: 'View and star skills-management on GitHub',
    })
    expect(link.getAttribute('href')).toBe('https://github.com/YPQuinn/skills-management')
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.querySelector('button')).toBeNull()
    expect(await screen.findByText('42')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith(
      'https://api.github.com/repos/YPQuinn/skills-management',
    )
  })

  it('keeps the repository link usable when the star count cannot be loaded', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))

    render(
      <GitHubStarsButton
        repo={repository}
        accessibleLabel="View and star skills-management on GitHub"
      />,
    )

    expect(await screen.findByText('0')).toBeTruthy()
    expect(
      screen.getByRole('link', { name: 'View and star skills-management on GitHub' }),
    ).toBeTruthy()
  })
})
