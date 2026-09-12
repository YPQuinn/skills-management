import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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
      { cache: 'no-store' },
    )
  })

  it('refreshes the count when the window regains focus', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ stargazers_count: 1 }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ stargazers_count: 2 }) })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <GitHubStarsButton
        repo={repository}
        accessibleLabel="View and star skills-management on GitHub"
      />,
    )

    expect(await screen.findByText('1')).toBeTruthy()
    fireEvent.focus(window)
    expect(await screen.findByText('2')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledTimes(2)
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

  it('keeps the last known count when a refresh fails', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ stargazers_count: 7 }) })
      .mockRejectedValueOnce(new Error('offline'))
    vi.stubGlobal('fetch', fetchMock)

    render(
      <GitHubStarsButton
        repo={repository}
        accessibleLabel="View and star skills-management on GitHub"
      />,
    )

    expect(await screen.findByText('7')).toBeTruthy()
    fireEvent.focus(window)
    expect(await screen.findByText('7')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
