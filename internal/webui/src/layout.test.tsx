import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ThemeProvider } from '@appica/ui-react/providers/theme-provider'
import { BackgroundPattern } from '@appica/ui-react/background-pattern'
import { Layout } from './layout'
import { LocaleProvider } from './locale-provider'

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

vi.mock('@appica/ui-react/background-pattern', () => ({
  BackgroundPattern: vi.fn((props: any) => <div>{props.children}</div>),
}))

function renderLayout() {
  return render(
    <MemoryRouter initialEntries={['/skills']}>
      <ThemeProvider>
        <LocaleProvider>
          <Layout>
            <h1>Skills</h1>
          </Layout>
        </LocaleProvider>
      </ThemeProvider>
    </MemoryRouter>,
  )
}

describe('Layout background pattern', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('wraps the content panel in the Appica dots pattern with top/side borders and no spotlight', () => {
    renderLayout()

    const props = vi.mocked(BackgroundPattern).mock.calls[0][0]
    expect(props.variant).toBe('dots')
    // The spotlight brightens its dot layer behind page text, which drops muted
    // labels such as the inactive tab below the AA contrast threshold.
    expect(props.spotlight).toBeUndefined()
    const classes = (props.className ?? '').split(/\s+/)
    expect(classes).toContain('mx-4')
    expect(classes).toContain('md:mx-6')
    expect(classes).toContain('flex-1')
    expect(classes).toContain('flex-col')
    expect(classes).not.toContain('overflow-hidden')
    expect(classes).toContain('rounded-t-[2rem]')
    expect(classes).toContain('border-x')
    expect(classes).toContain('border-t')
    expect(classes).toContain('border-border')
    expect(classes).not.toContain('border-b')
    expect(classes).not.toContain('border')
  })

  it('renders the Skill Manager mark next to the product name', () => {
    renderLayout()

    const title = screen.getByText('Skill Manager')
    const mark = title.previousElementSibling
    expect(mark?.tagName).toBe('IMG')
    expect(mark?.getAttribute('src')).toBe('/favicon.svg')
  })

  it('keeps the main navigation accessible', () => {
    renderLayout()

    expect(screen.getByRole('navigation', { name: 'Main navigation' })).toBeTruthy()
    for (const label of ['Skills', 'Sources', 'Groups', 'Targets']) {
      const link = screen.getByRole('link', { name: label })
      expect(link).toBeTruthy()
      expect(link.querySelector('svg[data-icon="start"]')).toBeTruthy()
    }
  })

  it('links to this project through the GitHub Stars button', () => {
    renderLayout()

    const link = screen.getByRole('link', {
      name: 'View and star skills-management on GitHub',
    })
    expect(link.getAttribute('href')).toBe('https://github.com/YPQuinn/skills-management')
  })

  it('keeps the centered nav from becoming a vertical scroll container', () => {
    renderLayout()

    const nav = screen.getByRole('navigation', { name: 'Main navigation' })
    const list = nav.querySelector('[data-slot="navigation-list"]')
    expect(list).toBeTruthy()
    const classes = (list?.className ?? '').split(/\s+/)
    expect(classes).toContain('max-md:overflow-x-auto')
    expect(classes).toContain('max-md:overflow-y-hidden')
    expect(classes).toContain('[&>li]:shrink-0')
    expect(classes).toContain('[&_a]:whitespace-nowrap')
    expect(classes).not.toContain('overflow-x-auto')
  })

  it('renders a sticky shell wrapping the translucent header and visual frame layer', () => {
    renderLayout()

    const shell = screen.getByTestId('sticky-shell')
    const shellClasses = shell.className.split(/\s+/)
    expect(shellClasses).toContain('sticky')
    expect(shellClasses).toContain('top-0')
    expect(shellClasses).toContain('z-20')

    const header = screen.getByRole('banner')
    expect(shell.contains(header)).toBe(true)
    const headerClasses = header.className.split(/\s+/)
    expect(headerClasses).toContain('bg-background/75')
    expect(headerClasses).toContain('backdrop-blur-lg')
    expect(headerClasses).not.toContain('border-b')

    const frame = screen.getByTestId('sticky-frame')
    expect(shell.contains(frame)).toBe(true)
    expect(frame.getAttribute('aria-hidden')).toBe('true')
    const frameClasses = frame.className.split(/\s+/)
    expect(frameClasses).toContain('pointer-events-none')
    expect(frameClasses).toContain('h-12')
    expect(frameClasses).toContain('-mb-12')
    expect(frameClasses).toContain('rounded-t-[2rem]')
    expect(frameClasses).toContain('border-x')
    expect(frameClasses).toContain('border-t')
    expect(frameClasses).toContain('border-border')
    expect(frameClasses).not.toContain('border-b')
  })
})
