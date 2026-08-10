import { useLocation, Link } from 'react-router-dom'
import { Navigation, NavigationList, NavigationItem, NavigationLink } from '@appica/ui-react/navigation'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { useTheme } from '@appica/ui-react/hooks/use-theme'

type ThemeChoice = 'system' | 'light' | 'dark'

export function Layout({ children }: { children: React.ReactNode }) {
  const { setTheme, theme, mounted } = useTheme()
  const location = useLocation()

  // Extract top-level path for navigation active state
  const currentPath = location.pathname.split('/')[1] || 'skills'

  return (
    <div className="min-h-dvh flex flex-col bg-background text-foreground">
      <header className="border-b border-border px-6 py-3 flex items-center justify-between gap-4">
        <div className="flex gap-4 md:gap-8 items-center min-w-0">
          <div className="font-semibold text-lg shrink-0">Skill Manager</div>
          <Navigation aria-label="Main" activeLink={currentPath} className="max-w-full">
            <NavigationList className="scrollbar-none overflow-x-auto [&::-webkit-scrollbar]:hidden">
              <NavigationItem>
                <NavigationLink value="skills" render={<Link to="/skills" />}>
                  Skills
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="sources" render={<Link to="/sources" />}>
                  Sources
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="groups" render={<Link to="/groups" />}>
                  Groups
                </NavigationLink>
              </NavigationItem>
              <NavigationItem>
                <NavigationLink value="targets" render={<Link to="/targets" />}>
                  Targets
                </NavigationLink>
              </NavigationItem>
            </NavigationList>
          </Navigation>
        </div>

        {mounted && (
          <Select value={theme ?? 'system'} onValueChange={(val) => setTheme(val as ThemeChoice)}>
            <SelectTrigger className="w-[120px]" aria-label="Theme">
              <SelectValue placeholder="System" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="system">System</SelectItem>
              <SelectItem value="light">Light</SelectItem>
              <SelectItem value="dark">Dark</SelectItem>
            </SelectContent>
          </Select>
        )}
      </header>
      <main className="flex-1 p-8 max-w-6xl w-full mx-auto">
        {children}
      </main>
    </div>
  )
}
