import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, beforeEach, vi } from 'vitest'
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

vi.mock('@appica/ui-react/scroll-area', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@appica/ui-react/scroll-area')>()
  return {
    ...actual,
    ScrollArea: (props: any) => (
      <div data-testid="mock-scroll-area" data-orientation={props.orientation}>
        {actual.ScrollArea ? actual.ScrollArea(props) : props.children}
      </div>
    ),
  }
})

describe('Localized Accessibility Labels & TableCaptions (English & Chinese)', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.lang = 'en'
    document.title = 'Skill Manager'
    vi.restoreAllMocks()
  })

  it('renders Sources Index TableCaption and Add Source Kind placeholder in English', async () => {
    localStorage.setItem('locale', 'en')

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) return new Response(JSON.stringify({ state: 'ready' }))
      if (urlStr.endsWith('/api/v1/sources')) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: 1,
                name: 'source-one',
                kind: 'local',
                location: '/tmp/skills',
                available: true,
                stale: false,
                entry_count: 0,
              },
            ],
          }),
        )
      }
      return new Response('Not found', { status: 404 })
    })

    const user = userEvent.setup()

    render(
      <MemoryRouter initialEntries={['/sources/']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('navigation', { name: 'Main navigation' })).toBeTruthy()
    expect(await screen.findByRole('table', { name: 'Registered Sources' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Add Source' }))
    const combo = await screen.findByRole('combobox', { name: 'Kind' })
    expect(combo.textContent).toContain('Local directory')
    expect(combo.textContent).not.toMatch(/^\s*local\s*$/)
    await user.click(combo)
    expect(await screen.findByRole('option', { name: 'Local directory' })).toBeTruthy()
  })

  it('renders Sources Index TableCaption and Add Source Select placeholder in Chinese', async () => {
    localStorage.setItem('locale', 'zh-CN')

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) return new Response(JSON.stringify({ state: 'ready' }))
      if (urlStr.endsWith('/api/v1/sources')) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: 1,
                name: 'source-one',
                kind: 'local',
                location: '/tmp/skills',
                available: true,
                stale: false,
                entry_count: 0,
              },
            ],
          }),
        )
      }
      return new Response('Not found', { status: 404 })
    })

    const user = userEvent.setup()

    render(
      <MemoryRouter initialEntries={['/sources/']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('navigation', { name: '主导航' })).toBeTruthy()
    expect(await screen.findByRole('table', { name: '已注册来源' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: '添加来源' }))
    const combo = await screen.findByRole('combobox', { name: '类型' })
    expect(combo.textContent).toContain('本地目录')
    expect(combo.textContent).not.toMatch(/^\s*local\s*$/)
    await user.click(combo)
    expect(await screen.findByRole('option', { name: '本地目录' })).toBeTruthy()
  })

  it('renders Source Inventory TableCaption in Chinese', async () => {
    localStorage.setItem('locale', 'zh-CN')

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) return new Response(JSON.stringify({ state: 'ready' }))
      if (urlStr.endsWith('/api/v1/sources/1')) {
        return new Response(
          JSON.stringify({
            id: 1,
            name: 'source-one',
            kind: 'local',
            location: '/tmp/skills',
            available: true,
            stale: false,
            inventory: [{ relative_dir: 'skills/alpha', name: 'Alpha', description: 'Demo' }],
            issues: [],
          }),
        )
      }
      if (urlStr.endsWith('/api/v1/sources')) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: 1,
                name: 'source-one',
                kind: 'local',
                location: '/tmp/skills',
                available: true,
                stale: false,
                entry_count: 1,
              },
            ],
          }),
        )
      }
      if (urlStr.endsWith('/api/v1/skills')) return new Response(JSON.stringify({ items: [] }))
      return new Response('Not found', { status: 404 })
    })

    render(
      <MemoryRouter initialEntries={['/sources/source-one']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('navigation', { name: '主导航' })).toBeTruthy()
    expect(await screen.findByRole('table', { name: '来源可用技能' })).toBeTruthy()

    expect(screen.queryByText(/类型:/)).toBeNull()
    expect(screen.queryByText(/Ref:/)).toBeNull()
    await userEvent.setup().click(screen.getByRole('button', { name: '详情' }))
    expect(screen.getByText(/类型:/)).toBeTruthy()
    expect(screen.getByText('local')).toBeTruthy()
    expect(screen.getByText(/Ref:/)).toBeTruthy()
    expect(screen.queryByText(/子路径:/)).toBeNull()
  })
})
