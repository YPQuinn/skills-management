import { useEffect } from 'react'
import { useLocalStorage } from '@appica/ui-react/hooks/use-local-storage'
import {
  translate,
  getInitialLocale,
  formatLocaleTime,
  getErrorMessage as getErrMsg,
  type Locale,
  type DictionaryKey,
} from './locale-dictionary'
import { LocaleContext } from './locale-context'

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocale] = useLocalStorage<Locale>('locale', getInitialLocale(), {
    serializer: (v) => v,
    deserializer: (v) => (v === 'zh-CN' ? 'zh-CN' : 'en'),
  })

  useEffect(() => {
    document.documentElement.lang = locale
    document.title = locale === 'zh-CN' ? '技能管理器' : 'Skill Manager'
  }, [locale])

  const t = (key: DictionaryKey, params?: Record<string, string | number>) =>
    translate(locale, key, params)

  const formatTime = (value?: string) => formatLocaleTime(locale, value)

  const getErrorMessage = (err: unknown, fallbackKey?: DictionaryKey) => getErrMsg(err, t, fallbackKey)

  return (
    <LocaleContext.Provider value={{ locale, setLocale, t, formatTime, getErrorMessage }}>
      {children}
    </LocaleContext.Provider>
  )
}
