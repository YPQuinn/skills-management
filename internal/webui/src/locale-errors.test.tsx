import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import App from './App'
import { getErrorMessage, ApiError, translate } from './locale-dictionary'

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

function tZh(key: Parameters<typeof translate>[1], params?: Record<string, string | number>) {
  return translate('zh-CN', key, params)
}

function tEn(key: Parameters<typeof translate>[1], params?: Record<string, string | number>) {
  return translate('en', key, params)
}

describe('WebUI Client Fallbacks vs Server Verbatim Errors', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.lang = 'en'
    document.title = 'Skill Manager'
    vi.restoreAllMocks()
  })

  it('maps native browser TypeError/Error to localized Chinese fallback keys while preserving ApiError.message verbatim', () => {
    const nativeNetErr = new TypeError('Failed to fetch')
    const serverApiErr = new ApiError('Server raw error: DB write lock failed')

    // Status / Setup
    expect(getErrorMessage(nativeNetErr, tZh, 'errSetupFailed')).toBe('初始化失败')
    expect(getErrorMessage(serverApiErr, tZh, 'errSetupFailed')).toBe('Server raw error: DB write lock failed')

    // Source register / rescan
    expect(getErrorMessage(nativeNetErr, tZh, 'errRegisteringSourceFailed')).toBe('注册来源失败')
    expect(getErrorMessage(nativeNetErr, tZh, 'errRescanningSourceFailed')).toBe('重新扫描来源失败')
    expect(getErrorMessage(serverApiErr, tZh, 'errRescanningSourceFailed')).toBe(
      'Server raw error: DB write lock failed',
    )

    // Skill load / import / replace
    expect(getErrorMessage(nativeNetErr, tZh, 'errSkillNotFound')).toBe('未找到技能')
    expect(getErrorMessage(nativeNetErr, tZh, 'errImportingSkillsFailed')).toBe('导入技能失败')
    expect(getErrorMessage(serverApiErr, tZh, 'errImportingSkillsFailed')).toBe(
      'Server raw error: DB write lock failed',
    )

    // English verification
    expect(getErrorMessage(nativeNetErr, tEn, 'errRegisteringSourceFailed')).toBe(
      'Registering the Source failed',
    )
    expect(getErrorMessage(serverApiErr, tEn, 'errRegisteringSourceFailed')).toBe(
      'Server raw error: DB write lock failed',
    )
  })

  it('renders server raw error.message verbatim in status alert while translating status fallback in zh mode', async () => {
    localStorage.setItem('locale', 'zh-CN')

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      if (String(url).endsWith('/api/v1/status')) {
        return new Response(JSON.stringify({ error: { message: 'Server raw error: DB read failure' } }), {
          status: 500,
        })
      }
      return new Response('Not found', { status: 404 })
    })

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('服务器错误')).toBeTruthy()
      expect(screen.getByText('获取应用状态失败：Server raw error: DB read failure')).toBeTruthy()
    })
  })

  it('renders localized Chinese client setup error when setup fails with generic error', async () => {
    localStorage.setItem('locale', 'zh-CN')

    vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url)
      if (urlStr.endsWith('/api/v1/status')) return new Response(JSON.stringify({ state: 'uninitialized' }))
      if (urlStr.endsWith('/api/v1/setup')) return new Response(JSON.stringify({}), { status: 500 })
      return new Response('Not found', { status: 404 })
    })

    const user = userEvent.setup()

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByRole('button', { name: '初始化' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '初始化' }))

    expect(await screen.findByText('初始化失败')).toBeTruthy()
  })
})
