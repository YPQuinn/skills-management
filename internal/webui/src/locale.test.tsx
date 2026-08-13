import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { useLocale } from './locale-context'
import { LocaleProvider } from './locale-provider'
import { getInitialLocale, translate, formatLocaleTime } from './locale-dictionary'

// Mock matchMedia for Appica
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

function TestComponent() {
  const { locale, setLocale, t, formatTime } = useLocale()
  return (
    <div>
      <span data-testid="active-locale">{locale}</span>
      <span data-testid="translated-title">{t('skillsTitle')}</span>
      <span data-testid="formatted-time">{formatTime('2026-08-13T10:00:00Z')}</span>
      <button onClick={() => setLocale('zh-CN')}>Switch to Chinese</button>
      <button onClick={() => setLocale('en')}>Switch to English</button>
    </div>
  )
}

describe('WebUI Localization Core', () => {
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

  describe('getInitialLocale & Persistence', () => {
    it('selects stored locale if present in localStorage', () => {
      localStorage.setItem('locale', 'zh-CN')
      expect(getInitialLocale()).toBe('zh-CN')

      localStorage.setItem('locale', 'en')
      expect(getInitialLocale()).toBe('en')
    })

    it('persisted en preference overrides zh-* browser navigator language', () => {
      Object.defineProperty(navigator, 'language', { value: 'zh-CN', configurable: true })
      localStorage.setItem('locale', 'en')
      expect(getInitialLocale()).toBe('en')
    })

    it('defaults to zh-CN when no locale stored and navigator.language starts with zh', () => {
      Object.defineProperty(navigator, 'language', { value: 'zh-CN', configurable: true })
      expect(getInitialLocale()).toBe('zh-CN')

      Object.defineProperty(navigator, 'language', { value: 'zh-TW', configurable: true })
      expect(getInitialLocale()).toBe('zh-CN')
    })

    it('defaults to en for all other browser languages when no locale stored', () => {
      Object.defineProperty(navigator, 'language', { value: 'en-US', configurable: true })
      expect(getInitialLocale()).toBe('en')

      Object.defineProperty(navigator, 'language', { value: 'fr-FR', configurable: true })
      expect(getInitialLocale()).toBe('en')
    })
  })

  describe('Single Pass Regex Interpolation & Adversarial Input', () => {
    it('handles dynamic values containing {key} tokens, $&, and braces without corruption or re-substitution', () => {
      const result = translate('en', 'summaryText', {
        total: '10 {name} $&',
        imported: '2 {time}',
        replaced: '0',
        already_imported: '0',
        skipped_conflict: '0',
        failed: '0',
      })

      expect(result).toBe(
        'Total: 10 {name} $&, 2 {time} imported, 0 replaced, 0 already imported, 0 conflict, 0 failed.',
      )
    })
  })

  describe('Locale Date Formatting', () => {
    it('formats timestamps using the selected locale', () => {
      const enTime = formatLocaleTime('en', '2026-08-13T10:00:00Z')
      const zhTime = formatLocaleTime('zh-CN', '2026-08-13T10:00:00Z')

      expect(enTime).not.toBe(zhTime)
      expect(zhTime).toMatch(/2026/)
    })
  })

  describe('Locale Provider & DOM Synchronization', () => {
    it('synchronizes html lang attribute and document title with selected locale', async () => {
      render(
        <LocaleProvider>
          <TestComponent />
        </LocaleProvider>,
      )

      expect(screen.getByTestId('active-locale').textContent).toBe('en')
      expect(document.documentElement.lang).toBe('en')
      expect(document.title).toBe('Skill Manager')

      fireEvent.click(screen.getByRole('button', { name: 'Switch to Chinese' }))

      await waitFor(() => {
        expect(screen.getByTestId('active-locale').textContent).toBe('zh-CN')
        expect(document.documentElement.lang).toBe('zh-CN')
        expect(document.title).toBe('技能管理器')
        expect(localStorage.getItem('locale')).toBe('zh-CN')
      })
    })
  })
})
