import { translationsEn } from './locale-dictionary-en'
import { translationsZhCN } from './locale-dictionary-zh'

export type Locale = 'en' | 'zh-CN'

export const translations = {
  en: translationsEn,
  'zh-CN': translationsZhCN,
} as const

export type DictionaryKey = keyof typeof translationsEn

export class ApiError extends Error {
  fallbackKey?: DictionaryKey
  params?: Record<string, string | number>

  constructor(
    message?: string,
    fallbackKey?: DictionaryKey,
    params?: Record<string, string | number>,
  ) {
    super(message || '')
    this.name = 'ApiError'
    this.fallbackKey = fallbackKey
    this.params = params
  }
}

export function translate(
  locale: Locale,
  key: DictionaryKey,
  params?: Record<string, string | number>,
): string {
  const template = translations[locale]?.[key] ?? translations.en[key] ?? String(key)
  if (!params) return template
  return template.replace(/\{([a-zA-Z0-9_]+)\}/g, (match, paramKey) => {
    if (Object.prototype.hasOwnProperty.call(params, paramKey)) {
      return String(params[paramKey])
    }
    return match
  })
}

export function getErrorMessage(
  err: unknown,
  t: (key: DictionaryKey, params?: Record<string, string | number>) => string,
  fallbackKey: DictionaryKey = 'unknownError',
): string {
  if (err instanceof ApiError) {
    if (err.message) return err.message
    if (err.fallbackKey) return t(err.fallbackKey, err.params)
  }
  if (typeof err === 'object' && err !== null) {
    const custom = err as { fallbackKey?: DictionaryKey; params?: Record<string, string | number> }
    if (custom.fallbackKey) return t(custom.fallbackKey, custom.params)
  }
  return t(fallbackKey)
}

export function getInitialLocale(): Locale {
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      const stored = window.localStorage.getItem('locale')
      if (stored === 'en' || stored === 'zh-CN') return stored
    } catch {}
  }
  if (typeof navigator !== 'undefined' && navigator.language) {
    const lang = navigator.language.toLowerCase()
    if (lang.startsWith('zh')) {
      return 'zh-CN'
    }
  }
  return 'en'
}

export function formatLocaleTime(locale: Locale, value?: string): string {
  if (!value) return translate(locale, 'timeNever')
  const d = new Date(value)
  return Number.isNaN(d.getTime())
    ? value
    : d.toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')
}

const DAY_MS = 86_400_000

export function formatRelativeTime(locale: Locale, value?: string, nowMs = Date.now()): string {
  if (!value) return translate(locale, 'timeNever')
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  const diffMs = d.getTime() - nowMs
  if (Math.abs(diffMs) >= 30 * DAY_MS) return formatLocaleTime(locale, value)
  const tag = locale === 'zh-CN' ? 'zh-CN' : 'en'
  const rtf = new Intl.RelativeTimeFormat(tag, { numeric: 'auto' })
  const abs = Math.abs(diffMs)
  if (abs < 60_000) return rtf.format(Math.round(diffMs / 1000), 'second')
  if (abs < 3_600_000) return rtf.format(Math.round(diffMs / 60_000), 'minute')
  if (abs < DAY_MS) return rtf.format(Math.round(diffMs / 3_600_000), 'hour')
  return rtf.format(Math.round(diffMs / DAY_MS), 'day')
}
