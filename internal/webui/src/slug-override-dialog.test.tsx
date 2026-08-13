import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { SlugOverrideDialog } from './slug-override-dialog'
import { LocaleProvider } from './locale-provider'

const triggerName = 'Slug override for Alpha (skills/alpha)'

function SlugHarness({ initialValue = 'saved-slug' }: { initialValue?: string }) {
  const [value, setValue] = useState(initialValue)
  return (
    <SlugOverrideDialog
      name="Alpha"
      relativeDir="skills/alpha"
      value={value}
      disabled={false}
      onSave={setValue}
    />
  )
}

function renderDialog(initialValue = 'saved-slug') {
  return render(<SlugHarness initialValue={initialValue} />)
}

function dialogInput(dialog: HTMLElement): HTMLInputElement {
  return within(dialog).getByRole('textbox') as HTMLInputElement
}

describe('SlugOverrideDialog', () => {
  afterEach(() => cleanup())

  it('opens through a trigger with dialog trigger semantics', async () => {
    const user = userEvent.setup()
    renderDialog()

    const trigger = screen.getByRole('button', { name: triggerName })
    expect(trigger.tagName).toBe('BUTTON')
    expect(trigger.getAttribute('aria-haspopup')).toBe('dialog')
    expect(trigger.getAttribute('aria-expanded')).toBe('false')

    await user.click(trigger)
    expect(await screen.findByRole('dialog')).toBeTruthy()
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
  })

  it('describes which skill the override applies to by name and relative dir', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: triggerName }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Set a custom slug for "Alpha" (skills/alpha). Leave empty to use the default.')).toBeTruthy()
  })

  it('disambiguates same-named skills in different dirs on the trigger and in the description', async () => {
    const user = userEvent.setup()
    render(
      <>
        <SlugOverrideDialog name="Alpha" relativeDir="skills/alpha" value="" disabled={false} onSave={() => {}} />
        <SlugOverrideDialog name="Alpha" relativeDir="other/alpha" value="" disabled={false} onSave={() => {}} />
      </>,
    )

    const first = screen.getByRole('button', { name: 'Slug override for Alpha (skills/alpha)' })
    const second = screen.getByRole('button', { name: 'Slug override for Alpha (other/alpha)' })
    expect(first).not.toBe(second)

    await user.click(first)
    const firstDialog = await screen.findByRole('dialog')
    expect(within(firstDialog).getByText(/\(skills\/alpha\)/)).toBeTruthy()
    expect(within(firstDialog).queryByText(/\(other\/alpha\)/)).toBeNull()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(second)
    const secondDialog = await screen.findByRole('dialog')
    expect(within(secondDialog).getByText(/\(other\/alpha\)/)).toBeTruthy()
    expect(within(secondDialog).queryByText(/\(skills\/alpha\)/)).toBeNull()
  })

  it('restores the saved value when reopened after Escape', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: triggerName }))
    const dialog = await screen.findByRole('dialog')
    const input = dialogInput(dialog)
    expect(input.value).toBe('saved-slug')

    await user.clear(input)
    await user.type(input, 'unsaved-draft')
    expect(input.value).toBe('unsaved-draft')

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(screen.getByRole('button', { name: triggerName }))
    const reopened = await screen.findByRole('dialog')
    expect(dialogInput(reopened).value).toBe('saved-slug')
  })

  it('restores the saved value when reopened after Cancel', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: triggerName }))
    const dialog = await screen.findByRole('dialog')
    const input = dialogInput(dialog)
    await user.clear(input)
    await user.type(input, 'temp-draft')

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(screen.getByRole('button', { name: triggerName }))
    const reopened = await screen.findByRole('dialog')
    expect(dialogInput(reopened).value).toBe('saved-slug')
  })

  it('saves the draft and reopens from the newly saved value', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: triggerName }))
    const dialog = await screen.findByRole('dialog')
    const input = dialogInput(dialog)
    await user.clear(input)
    await user.type(input, 'new-slug')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.getByRole('button', { name: triggerName }).textContent).toContain('new-slug')

    await user.click(screen.getByRole('button', { name: triggerName }))
    const reopened = await screen.findByRole('dialog')
    expect(dialogInput(reopened).value).toBe('new-slug')
  })

  it('does not open when disabled', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(<SlugOverrideDialog name="Alpha" relativeDir="skills/alpha" value="saved-slug" disabled onSave={onSave} />)

    const trigger = screen.getByRole('button', { name: triggerName })
    expect((trigger as HTMLButtonElement).disabled).toBe(true)
    await user.click(trigger)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(onSave).not.toHaveBeenCalled()
  })
})

describe('SlugOverrideDialog (Chinese)', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('locale', 'zh-CN')
  })

  afterEach(() => cleanup())

  it('localizes the trigger name, description, and dialog close label', async () => {
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <SlugOverrideDialog name="Alpha" relativeDir="skills/alpha" value="" disabled={false} onSave={() => {}} />
      </LocaleProvider>,
    )

    const trigger = screen.getByRole('button', { name: '为 Alpha（skills/alpha）设置标识覆盖' })
    await user.click(trigger)
    const dialog = await screen.findByRole('dialog')

    expect(within(dialog).getByText('为 "Alpha"（skills/alpha）设置自定义标识。留空则使用默认标识。')).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: '关闭' })).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: '保存' })).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: '取消' })).toBeTruthy()
  })
})
