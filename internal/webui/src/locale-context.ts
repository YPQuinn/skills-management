import { createContext, useContext } from 'react'
import {
  translate,
  formatLocaleTime,
  formatRelativeTime,
  getErrorMessage as getErrMsg,
  type Locale,
  type DictionaryKey,
} from './locale-dictionary'

export interface LocaleContextType {
  locale: Locale
  setLocale: (locale: Locale) => void
  t: (key: DictionaryKey, params?: Record<string, string | number>) => string
  formatTime: (value?: string) => string
  formatRelativeTime: (value?: string) => string
  getErrorMessage: (err: unknown, fallbackKey?: DictionaryKey) => string
}

export const defaultContext: LocaleContextType = {
  locale: 'en',
  setLocale: () => {},
  t: (key, params) => translate('en', key, params),
  formatTime: (value) => formatLocaleTime('en', value),
  formatRelativeTime: (value) => formatRelativeTime('en', value),
  getErrorMessage: (err, fallbackKey) => getErrMsg(err, (k, p) => translate('en', k, p), fallbackKey),
}

export const LocaleContext = createContext<LocaleContextType>(defaultContext)

export function useLocale(): LocaleContextType {
  return useContext(LocaleContext)
}
