import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import App from './App'

if (typeof window !== 'undefined') {
  window.matchMedia =
    window.matchMedia ||
    function () {
      return {
        matches: false,
        addListener: function () {},
        removeListener: function () {},
        addEventListener: function () {},
        removeEventListener: function () {},
        dispatchEvent: function () {
          return false
        },
      }
    }
}

describe('WebUI Localization Behavior - No Extra Fetch / Delayed Status', () => {
  const originalLanguage = navigator.language

  beforeEach(() => {
    localStorage.clear()
    document.documentElement.lang = 'en'
    document.title = 'Skill Manager'
    vi.restoreAllMocks()
  })

  afterEach(() => {
    Object.defineProperty(navigator, 'language', {
      value: originalLanguage,
      configurable: true,
    })
  })

  it('does not issue a second request or blank out on delayed app status request when switching locale', async () => {
    let statusFetchCount = 0
    let resolveStatus!: (res: Response) => void
    const statusPromise = new Promise<Response>((r) => {
      resolveStatus = r
    })

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) {
        statusFetchCount++
        return statusPromise
      }
      if (urlStr.endsWith('/api/v1/skills')) {
        return new Response(JSON.stringify({ items: [] }))
      }
      return new Response('Not found', { status: 404 })
    })

    localStorage.setItem('locale', 'zh-CN')

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    expect(screen.getByRole('status', { name: '正在检查应用状态' })).toBeTruthy()
    expect(statusFetchCount).toBe(1)

    // Resolve status fetch as ready
    resolveStatus(new Response(JSON.stringify({ state: 'ready' })))

    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: '主导航' })).toBeTruthy()
    })

    expect(statusFetchCount).toBe(1)
  })

  it('does not refetch or blank out Source detail page on locale change', async () => {
    let fetchCount = 0
    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) {
        return new Response(JSON.stringify({ state: 'ready' }))
      }
      if (urlStr.endsWith('/api/v1/sources/1')) {
        fetchCount++
        return new Response(
          JSON.stringify({
            id: 1,
            name: 'source-one',
            kind: 'local',
            location: '/tmp/skills',
            available: true,
            stale: false,
            inventory: [],
            issues: [],
          }),
        )
      }
      if (urlStr.endsWith('/api/v1/sources')) {
        return new Response(JSON.stringify({ items: [{ id: 1, name: 'source-one' }] }))
      }
      if (urlStr.endsWith('/api/v1/skills')) {
        return new Response(JSON.stringify({ items: [] }))
      }
      return new Response('Not found', { status: 404 })
    })

    render(
      <MemoryRouter initialEntries={['/sources/source-one']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'source-one' })).toBeTruthy()
    const initialFetchCount = fetchCount

    // Switch language to zh-CN via Language Select
    const user = userEvent.setup()
    const langTrigger = screen.getByRole('combobox', { name: 'Language' })
    await user.click(langTrigger)

    const optionZh = await screen.findByRole('option', { name: '简体中文' })
    await user.click(optionZh)

    await waitFor(() => {
      expect(screen.getByText('重新检查')).toBeTruthy()
    })

    // Ensure no extra fetch was issued and page did not blank out
    expect(fetchCount).toBe(initialFetchCount)
    expect(screen.queryByRole('status', { name: 'Loading source' })).toBeNull()
  })

  it('does not refetch or blank out Skill detail page on locale change', async () => {
    let fetchCount = 0
    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) {
        return new Response(JSON.stringify({ state: 'ready' }))
      }
      if (urlStr.endsWith('/api/v1/skills/101')) {
        fetchCount++
        return new Response(
          JSON.stringify({
            id: 101,
            slug: 'alpha',
            name: 'Alpha Skill',
            description: 'Demo skill',
          }),
        )
      }
      if (urlStr.endsWith('/api/v1/skills')) {
        return new Response(
          JSON.stringify({ items: [{ id: 101, slug: 'alpha', name: 'Alpha Skill' }] }),
        )
      }
      return new Response('Not found', { status: 404 })
    })

    render(
      <MemoryRouter initialEntries={['/skills/alpha']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: 'Alpha Skill' })).toBeTruthy()
    const initialFetchCount = fetchCount

    // Switch language to zh-CN via Language Select
    const user = userEvent.setup()
    const langTrigger = screen.getByRole('combobox', { name: 'Language' })
    await user.click(langTrigger)

    const optionZh = await screen.findByRole('option', { name: '简体中文' })
    await user.click(optionZh)

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: '概览' })).toBeTruthy()
    })

    // Ensure fetchCount did not increase
    expect(fetchCount).toBe(initialFetchCount)
  })
})
