import { useState } from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SourceInventory } from './source-inventory'
import { setupMatchMedia } from './source-fixtures'
import type { SourceEntry } from './source-api'

setupMatchMedia()

vi.mock('@appica/ui-react/scroll-area', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@appica/ui-react/scroll-area')>()
  return {
    ...actual,
    ScrollArea: (props: any) => (
      <div data-testid="mock-scroll-area" data-orientation={props.orientation}>
        {actual.ScrollArea ? actual.ScrollArea(props) : props.children}
      </div>
    ),
  }
})

function makeEntries(count: number, nameFor?: (index: number) => string): SourceEntry[] {
  return Array.from({ length: count }, (_, index) => ({
    relative_dir: `skills/entry-${index + 1}`,
    name: nameFor ? nameFor(index) : `Skill ${index + 1}`,
    description: `Description ${index + 1}`,
  }))
}

function InventoryHarness({ inventory }: { inventory: SourceEntry[] }) {
  const [selectedDirs, setSelectedDirs] = useState<string[]>([])
  return (
    <SourceInventory
      inventory={inventory}
      available
      rescanning={false}
      onRescan={() => {}}
      boundSkillMap={new Map()}
      selectedDirs={selectedDirs}
      setSelectedDirs={setSelectedDirs}
      slugOverrides={{}}
      onSlugOverrideChange={() => {}}
      allowLarge={false}
      setAllowLarge={() => {}}
      importing={false}
      importResult={null}
      onImport={() => {}}
    />
  )
}

function renderInventory(inventory: SourceEntry[]) {
  return render(
    <MemoryRouter>
      <InventoryHarness inventory={inventory} />
    </MemoryRouter>,
  )
}

function currentPageNumber(): string {
  return document.querySelector('[aria-current="page"]')?.textContent?.trim() ?? ''
}

describe('SourceInventory pagination and search', () => {
  afterEach(() => cleanup())

  it('paginates 10 entries per page and switches pages', () => {
    renderInventory(makeEntries(12))
    expect(screen.getByText('Skill 1')).toBeTruthy()
    expect(screen.getByText('Skill 10')).toBeTruthy()
    expect(screen.queryByText('Skill 11')).toBeNull()
    expect(screen.queryByText('Skill 12')).toBeNull()
    expect(screen.getByRole('navigation', { name: 'Pagination' })).toBeTruthy()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))

    expect(screen.getByText('Skill 11')).toBeTruthy()
    expect(screen.getByText('Skill 12')).toBeTruthy()
    expect(screen.queryByText('Skill 1')).toBeNull()
  })

  it('hides pagination when the inventory fits on one page', () => {
    renderInventory(makeEntries(10))
    expect(screen.getByText('Skill 10')).toBeTruthy()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })

  it('filters by trimmed, case-insensitive name substring', () => {
    renderInventory([
      ...makeEntries(1, () => 'React Hooks Guide'),
      ...makeEntries(1, () => 'react router basics'),
      ...makeEntries(1, () => 'Testing Patterns'),
    ])

    fireEvent.change(screen.getByLabelText('Search inventory skills by name'), { target: { value: '  ReAcT  ' } })

    expect(screen.getByText('React Hooks Guide')).toBeTruthy()
    expect(screen.getByText('react router basics')).toBeTruthy()
    expect(screen.queryByText('Testing Patterns')).toBeNull()
  })

  it('returns to page 1 when searching from a later page and when clearing', () => {
    renderInventory(makeEntries(12, (index) => `Guide Skill ${index + 1}`))

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(currentPageNumber()).toBe('2')

    fireEvent.change(screen.getByLabelText('Search inventory skills by name'), { target: { value: 'guide' } })
    expect(currentPageNumber()).toBe('1')
    expect(screen.getByText('Guide Skill 1')).toBeTruthy()
    expect(screen.queryByText('Guide Skill 12')).toBeNull()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(currentPageNumber()).toBe('2')

    fireEvent.click(screen.getByRole('button', { name: 'Clear input' }))
    expect(currentPageNumber()).toBe('1')
    expect(screen.getByText('Guide Skill 1')).toBeTruthy()
  })

  it('shows a localized no-results state and keeps the search box visible', () => {
    renderInventory(makeEntries(12))

    fireEvent.change(screen.getByLabelText('Search inventory skills by name'), { target: { value: 'zzz-no-match' } })

    expect(screen.getByText('No skills match your search.')).toBeTruthy()
    expect(screen.getByLabelText('Search inventory skills by name')).toBeTruthy()
    expect(screen.queryByRole('table', { name: 'Source inventory' })).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })

  it('keeps the emptyInventory state without search or pagination for an empty inventory', () => {
    renderInventory([])

    expect(screen.getByText('No valid Skills discovered.')).toBeTruthy()
    expect(screen.queryByLabelText('Search inventory skills by name')).toBeNull()
    expect(screen.queryByText('No skills match your search.')).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).toBeNull()
  })

  it('collapses long page ranges with an ellipsis and keeps the current page non-interactive', () => {
    renderInventory(makeEntries(125))

    const nav = screen.getByRole('navigation', { name: 'Pagination' })
    expect(nav.querySelectorAll('[data-slot="pagination-ellipsis"]').length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: /^Go to page \d+$/ }).length).toBeLessThanOrEqual(5)

    const active = document.querySelector('[aria-current="page"]')
    expect(active?.textContent?.trim()).toBe('1')
    expect(active?.hasAttribute('href')).toBe(false)
    expect(screen.queryByRole('link', { name: 'Go to page 1' })).toBeNull()
    expect(active?.getAttribute('tabindex')).toBe('-1')

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 13' }))
    expect(currentPageNumber()).toBe('13')
    expect(screen.getByRole('link', { name: 'Go to page 1' })).toBeTruthy()
    expect(nav.querySelectorAll('[data-slot="pagination-ellipsis"]').length).toBeGreaterThan(0)
  })

  it('disables prev/next links at the bounds', () => {
    renderInventory(makeEntries(12))

    expect(screen.getByRole('link', { name: 'Go to previous page' }).hasAttribute('aria-disabled')).toBe(true)
    expect(screen.getByRole('link', { name: 'Go to next page' }).hasAttribute('aria-disabled')).toBe(false)

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(screen.getByRole('link', { name: 'Go to previous page' }).hasAttribute('aria-disabled')).toBe(false)
    expect(screen.getByRole('link', { name: 'Go to next page' }).hasAttribute('aria-disabled')).toBe(true)
  })

  it('selects only the visible page from the header and keeps selections on other pages', async () => {
    const user = userEvent.setup()
    renderInventory(makeEntries(12))

    await user.click(screen.getByRole('checkbox', { name: 'Select all on this page (10)' }))
    expect(screen.getByRole('button', { name: 'Import selected (10)' })).toBeTruthy()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(screen.getByRole('checkbox', { name: 'Select all on this page (2)' }).getAttribute('aria-checked')).toBe('false')

    await user.click(screen.getByRole('checkbox', { name: 'Select Skill 11' }))
    expect(screen.getByRole('checkbox', { name: 'Select all on this page (2)' }).getAttribute('aria-checked')).toBe('mixed')

    await user.click(screen.getByRole('checkbox', { name: 'Select all on this page (2)' }))
    expect(screen.getByRole('button', { name: 'Import selected (12)' })).toBeTruthy()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 1' }))
    expect(screen.getByRole('checkbox', { name: 'Select all on this page (10)' }).getAttribute('aria-checked')).toBe('true')

    await user.click(screen.getByRole('checkbox', { name: 'Select all on this page (10)' }))
    expect(screen.getByRole('button', { name: 'Import selected (2)' })).toBeTruthy()

    fireEvent.click(screen.getByRole('link', { name: 'Go to page 2' }))
    expect(screen.getByRole('checkbox', { name: 'Select all on this page (2)' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('checkbox', { name: 'Select Skill 11' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('checkbox', { name: 'Select Skill 12' }).getAttribute('aria-checked')).toBe('true')
  })

  it('keeps selections filtered out of view and scopes the header to visible entries', async () => {
    const user = userEvent.setup()
    renderInventory(makeEntries(12))

    await user.click(screen.getByRole('checkbox', { name: 'Select Skill 1' }))
    expect(screen.getByRole('button', { name: 'Import selected (1)' })).toBeTruthy()

    fireEvent.change(screen.getByLabelText('Search inventory skills by name'), { target: { value: 'Skill 2' } })
    expect(screen.getByText('Skill 2')).toBeTruthy()
    expect(screen.queryByText('Skill 1')).toBeNull()

    // the hidden selection still counts; the header reflects only the visible row
    expect(screen.getByRole('button', { name: 'Import selected (1)' })).toBeTruthy()
    expect(screen.getByRole('checkbox', { name: 'Select all on this page (1)' }).getAttribute('aria-checked')).toBe('false')

    fireEvent.click(screen.getByRole('button', { name: 'Clear input' }))
    expect(screen.getByRole('checkbox', { name: 'Select Skill 1' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('button', { name: 'Import selected (1)' })).toBeTruthy()
  })
})
