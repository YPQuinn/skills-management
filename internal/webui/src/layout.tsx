import { useLocation, Link } from 'react-router-dom'
import { Navigation, NavigationList, NavigationItem, NavigationLink } from '@appica/ui-react/navigation'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { useTheme } from '@appica/ui-react/hooks/use-theme'
import { useLocale } from './locale-context'
import type { Locale } from './locale-dictionary'

type ThemeChoice = 'system' | 'light' | 'dark'

export function Layout({ children }: { children: React.ReactNode }) {
  const { setTheme, theme, mounted } = useTheme()
  const { locale, setLocale, t } = useLocale()
  const location = useLocation()

  // Extract top-level path for navigation active state
  const currentPath = location.pathname.split('/')[1] || 'skills'

  return (
    <div className="min-h-dvh flex flex-col bg-background text-foreground">
      <header className="border-b border-border px-6 py-3 flex items-center justify-between gap-4">
        <div className="flex gap-4 md:gap-8 items-center min-w-0">
          <div className="font-semibold text-lg shrink-0">{t('appTitle')}</div>
          <Navigation aria-label={t('ariaMainNavigation')} activeLink={currentPath} className="max-w-full">
            <NavigationList className="scrollbar-none overflow-x-auto [&::-webkit-scrollbar]:hidden">
              <NavigationItem>
                <NavigationLink value="skills" render={<Link to="/skills" />}>
                  {t('navSkills')}
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="sources" render={<Link to="/sources" />}>
                  {t('navSources')}
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="groups" render={<Link to="/groups" />}>
                  {t('navGroups')}
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="targets" render={<Link to="/targets" />}>
                  {t('navTargets')}
                </NavigationLink>
              </NavigationItem>
            </NavigationList>
          </Navigation>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {mounted && (
            <Select value={theme ?? 'system'} onValueChange={(val) => setTheme(val as ThemeChoice)}>
              <SelectTrigger className="w-[110px]" aria-label={t('ariaTheme')}>
                <SelectValue placeholder={t('themeSystem')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">{t('themeSystem')}</SelectItem>
                <SelectItem value="light">{t('themeLight')}</SelectItem>
                <SelectItem value="dark">{t('themeDark')}</SelectItem>
              </SelectContent>
            </Select>
          )}

          <Select value={locale} onValueChange={(val) => setLocale(val as Locale)}>
            <SelectTrigger className="w-[120px]" aria-label={t('ariaLanguage')}>
              <SelectValue placeholder={locale === 'zh-CN' ? '简体中文' : 'English'} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="en">{t('langEn')}</SelectItem>
              <SelectItem value="zh-CN">{t('langZhCN')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </header>
      <main className="flex-1 p-8 max-w-6xl w-full mx-auto">
        {children}
      </main>
    </div>
  )
}
