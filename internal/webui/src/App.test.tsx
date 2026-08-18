import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { ThemeProvider } from '@appica/ui-react/providers/theme-provider'

// Mock matchMedia for Appica
window.matchMedia = window.matchMedia || function() {
  return {
    matches: false,
    addListener: function() {},
    removeListener: function() {},
    addEventListener: function() {},
    removeEventListener: function() {},
    dispatchEvent: function() {}
  }
}

function renderApp() {
  return render(
    <BrowserRouter>
      <ThemeProvider>
        <App />
      </ThemeProvider>
    </BrowserRouter>
  )
}

function mockResponse(body: unknown, ok = true): Response {
  return new Response(JSON.stringify(body), {
    status: ok ? 200 : 400,
    headers: { 'Content-Type': 'application/json' }
  })
}

describe('App routing and state', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.history.pushState({}, '', '/')
    window.fetch = vi.fn().mockImplementation(async () => mockResponse({ state: 'ready' }))
  })

  afterEach(() => {
    cleanup()
  })

  it('renders setup form on uninitialized state', async () => {
    vi.mocked(window.fetch).mockImplementation(async () => mockResponse({ state: 'uninitialized' }))
    
    renderApp()
    
    expect(await screen.findByText('Welcome to Skill Manager')).toBeTruthy()
  })

  it('redirects to skills page on ready state', async () => {
    vi.mocked(window.fetch).mockImplementation(async () => mockResponse({ state: 'ready' }))
    
    renderApp()
    
    expect(await screen.findByRole('heading', { name: 'Skills' })).toBeTruthy()
    // Verify navigation links are present
    expect(screen.getByRole('link', { name: 'Sources' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Groups' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Targets' })).toBeTruthy()
  })
  
  it('renders recover action on state_missing state', async () => {
    vi.mocked(window.fetch).mockImplementation(async () => mockResponse({ state: 'state_missing', message: 'Missing state file' }))

    renderApp()

    expect(await screen.findByText('Error: state_missing')).toBeTruthy()
    expect(screen.getByText('Missing state file')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Initialize' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Recover Store' })).toBeTruthy()
  })

  it('renders clear error on invalid state', async () => {
    vi.mocked(window.fetch).mockImplementation(async () => mockResponse({ state: 'invalid', message: 'Invalid DB schema' }))
    
    renderApp()
    
    expect(await screen.findByText('Error: invalid')).toBeTruthy()
    expect(screen.getByText('Invalid DB schema')).toBeTruthy()
  })

  it('renders a useful error surface on status-fetch failure', async () => {
    vi.mocked(window.fetch).mockRejectedValue(new Error('Network offline'))
    
    renderApp()
    
    expect(await screen.findByText('Server Error')).toBeTruthy()
    expect(screen.getByText(/Failed to fetch application status/)).toBeTruthy()
  })
})

describe('Deep links', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.fetch = vi.fn().mockImplementation(async () => mockResponse({ state: 'ready' }))
  })

  afterEach(() => {
    cleanup()
  })

  it('renders skills shell for nested skill routes', async () => {
    window.history.pushState({}, '', '/skills/example-skill?tab=synchronization')
    renderApp()
    expect(await screen.findByRole('heading', { name: 'Skills' })).toBeTruthy()
  })
  
  it('renders targets shell for nested target routes', async () => {
    window.history.pushState({}, '', '/targets/pi-user')
    renderApp()
    expect(await screen.findByRole('heading', { name: 'Targets' })).toBeTruthy()
  })
})

describe('Setup validation', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.history.pushState({}, '', '/')
    window.fetch = vi.fn().mockImplementation(async () => mockResponse({ state: 'uninitialized' }))
  })

  afterEach(() => {
    cleanup()
  })

  it('validates absolute paths client-side', async () => {
    vi.mocked(window.fetch).mockImplementation(async () => mockResponse({ state: 'uninitialized' }))
    
    renderApp()
    
    await screen.findByText('Welcome to Skill Manager')
    
    const input = screen.getByPlaceholderText('Leave blank for default')
    const button = screen.getByRole('button', { name: 'Initialize' })
    const user = userEvent.setup()
    
    await user.type(input, 'relative/path')
    await user.click(button)
    
    const calls = vi.mocked(window.fetch).mock.calls
    const setupCalls = calls.filter((call: Parameters<typeof window.fetch>) => call[0] === '/api/v1/setup')
    expect(setupCalls.length).toBe(0)
    expect(await screen.findByText('Path must be absolute (e.g. starting with "/")')).toBeTruthy()
  })

  it('submits exact JSON on success and redirects, asserting loading state', async () => {
    let resolveSetup: (value: Response) => void
    const setupPromise = new Promise<Response>((resolve) => {
      resolveSetup = resolve
    })

    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (url === '/api/v1/setup') {
        return setupPromise
      }
      return mockResponse({ state: 'uninitialized' })
    })
    
    renderApp()
    
    await screen.findByText('Welcome to Skill Manager')
    
    const input = screen.getByPlaceholderText('Leave blank for default')
    const button = screen.getByRole('button', { name: 'Initialize' })
    const user = userEvent.setup()
    
    await user.type(input, '/absolute/path')
    
    // Fire the click but don't await immediately to check loading state
    const clickPromise = user.click(button)
    
    // Assert loading state (button text changes and aria-disabled or disabled attribute is present)
    expect(await screen.findByText('Initializing...')).toBeTruthy()
    const loadingBtn = screen.getByRole('button', { name: /Initializing/ })
    expect(loadingBtn.hasAttribute('aria-disabled') || loadingBtn.hasAttribute('disabled')).toBe(true)

    // Resolve the setup request
    resolveSetup!(mockResponse({ state: 'ready' }))
    await clickPromise
    
    // Verify exact JSON body sent
    expect(window.fetch).toHaveBeenCalledWith('/api/v1/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ store_path: '/absolute/path' })
    })
    
    // Verify redirection to /skills
    expect(await screen.findByRole('heading', { name: 'Skills' })).toBeTruthy()
  })

  it('recovers a missing state database through setup', async () => {
    vi.mocked(window.fetch).mockImplementation(async (url: RequestInfo | URL) => {
      if (url === '/api/v1/setup') {
        return mockResponse({ state: 'ready', recovered: [{ slug: 'alpha' }] })
      }
      return mockResponse({ state: 'state_missing', message: 'Missing state file' })
    })

    renderApp()
    const button = await screen.findByRole('button', { name: 'Recover Store' })
    await userEvent.setup().click(button)

    expect(window.fetch).toHaveBeenCalledWith('/api/v1/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ recover_store: true }),
    })
    expect(await screen.findByRole('heading', { name: 'Skills' })).toBeTruthy()
  })
})
