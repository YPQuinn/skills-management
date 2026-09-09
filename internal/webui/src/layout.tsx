import { useLocation, Link } from 'react-router-dom'
import { BackgroundPattern } from '@appica/ui-react/background-pattern'
import { Navigation, NavigationList, NavigationItem, NavigationLink } from '@appica/ui-react/navigation'
import { Button, buttonVariants } from '@appica/ui-react/button'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroupLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
} from '@appica/ui-react/dropdown-menu'
import { useTheme } from '@appica/ui-react/hooks/use-theme'
import {
  Sparkles,
  BrandGithub,
  Settings,
  DeviceDesktop,
  Sun,
  Moon,
  Language,
} from '@appica/icons-react'
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
      <div data-testid="sticky-shell" className="sticky top-0 z-20">
        <header className="bg-background/75 backdrop-blur-lg px-6 py-3 flex items-center justify-between gap-4">
          <div className="flex items-center gap-2 font-bold text-lg text-foreground shrink-0">
            <Sparkles className="size-5 shrink-0 text-foreground" />
            <span>{t('appTitle')}</span>
          </div>

          <Navigation
            aria-label={t('ariaMainNavigation')}
            variant="line"
            activeLink={currentPath}
            className="flex-1 min-w-0 md:flex-initial md:absolute md:left-1/2 md:-translate-x-1/2"
          >
            <NavigationList className="max-md:overflow-x-auto max-md:overflow-y-hidden">
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

          <div className="flex items-center gap-2 shrink-0">
            <a
              href="https://github.com/"
              target="_blank"
              rel="noreferrer"
              className={buttonVariants({ variant: 'ghost', size: 'icon-md' })}
              aria-label={t('ariaGitHub')}
            >
              <BrandGithub />
            </a>

            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="primary"
                    size="icon-md"
                    className="rounded-xl"
                    aria-label={t('ariaSettings')}
                  >
                    <Settings />
                  </Button>
                }
              />
              <DropdownMenuContent align="end" className="min-w-48">
                <DropdownMenuRadioGroup
                  value={mounted ? (theme ?? 'system') : 'system'}
                  onValueChange={(val) => setTheme(val as ThemeChoice)}
                >
                  <DropdownMenuGroupLabel>{t('ariaTheme')}</DropdownMenuGroupLabel>
                  <DropdownMenuRadioItem value="system">
                    <DeviceDesktop data-icon="start" />
                    {t('themeSystem')}
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="light">
                    <Sun data-icon="start" />
                    {t('themeLight')}
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="dark">
                    <Moon data-icon="start" />
                    {t('themeDark')}
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>

                <DropdownMenuSeparator />

                <DropdownMenuRadioGroup
                  value={locale}
                  onValueChange={(val) => setLocale(val as Locale)}
                >
                  <DropdownMenuGroupLabel>{t('ariaLanguage')}</DropdownMenuGroupLabel>
                  <DropdownMenuRadioItem value="en">
                    <Language data-icon="start" />
                    {t('langEn')}
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="zh-CN">
                    <Language data-icon="start" />
                    {t('langZhCN')}
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <div
          data-testid="sticky-frame"
          className="pointer-events-none mx-4 md:mx-6 h-12 -mb-12 rounded-t-[2rem] border-x border-t border-border"
          aria-hidden="true"
        />
      </div>
      {/* Keep content outside rounded overflow clipping: Chrome can drop it when the masked spotlight repaints. */}
      <BackgroundPattern
        variant="dots"
        spotlight
        className="mx-4 md:mx-6 flex flex-1 flex-col rounded-t-[2rem] border-x border-t border-border"
      >
        <main className="flex-1 p-8 max-w-6xl w-full mx-auto">
          {children}
        </main>
      </BackgroundPattern>
    </div>
  )
}
