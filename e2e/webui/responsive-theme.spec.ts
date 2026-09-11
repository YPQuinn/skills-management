import { expect, test } from '@playwright/test'
import path from 'node:path'
import { expectImported, expectNoHorizontalOverflow, initializeStore, navTo, openCreateDialog, openSettings, startUI, writeSkillFixture } from './helpers'

test('desktop master-detail, narrow list/detail split, no overflow', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'wide')
    await navTo(page, 'Sources')
    await openCreateDialog(page, 'Add Source')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await page.getByRole('button', { name: 'Import all' }).click()
    await expectImported(page)

    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto(`${ui.url}/skills/wide`)
    await expect(page.getByRole('heading', { name: 'wide', level: 1 })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toBeVisible()
    // Rounded overflow clipping around the masked background can drop content during repaint.
    const background = page.locator('[data-slot="background-pattern"]')
    await expect(background).toHaveCSS('overflow-x', 'visible')
    await expect(background).toHaveCSS('overflow-y', 'visible')
    await expectNoHorizontalOverflow(page)
    const mainNavList = page.getByRole('navigation', { name: 'Main navigation' }).locator('[data-slot="navigation-list"]')
    await expect(mainNavList).toHaveCSS('overflow-x', 'visible')
    await expect(mainNavList).toHaveCSS('overflow-y', 'visible')
    expect(await page.evaluate(() => getComputedStyle(document.documentElement).scrollbarGutter)).toContain('stable')

    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`${ui.url}/skills`)
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toHaveCount(0)
    await expect(mainNavList).toHaveCSS('overflow-x', 'auto')
    await expect(mainNavList).toHaveCSS('overflow-y', 'hidden')
    await expectNoHorizontalOverflow(page)
    await page.getByRole('link', { name: 'wide', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'wide', level: 1 })).toBeVisible()
    await expect(page.getByRole('link', { name: 'All Skills' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toHaveCount(0)
    await expectNoHorizontalOverflow(page)
  } finally {
    await ui.stop()
  }
})

test('background decorations stay inside the rounded corners during pointer movement', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    for (const width of [1280, 390]) {
      await page.setViewportSize({ width, height: 800 })
      const panel = await page.locator('[data-slot="background-pattern"]').boundingBox()
      expect(panel).not.toBeNull()
      if (!panel) throw new Error('Background panel is missing')

      // These small squares are outside the 32px corner arcs, so they must match the plain margin.
      const margin = await page.screenshot({ clip: { x: panel.x - 8, y: panel.y + 1, width: 6, height: 6 } })
      for (const x of [panel.x + 1, panel.x + panel.width - 7]) {
        await page.mouse.move(width / 2, panel.y + 100)
        await page.mouse.move(x + 3, panel.y + 4, { steps: 5 })
        const corner = await page.screenshot({ clip: { x, y: panel.y + 1, width: 6, height: 6 } })
        expect(corner.equals(margin), `Dots leaked outside a rounded corner at width ${width}`).toBe(true)
      }
    }
  } finally {
    await ui.stop()
  }
})

test('theme system/light/dark persist and reduced-motion is honored', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.goto(ui.url + '/setup')
    await initializeStore(page)

    const theme = async () => page.evaluate(() => ({
      stored: localStorage.getItem('theme'),
      dark: document.documentElement.classList.contains('dark'),
    }))

    expect((await theme()).stored === null || (await theme()).stored === 'system').toBeTruthy()
    await expect.poll(async () => (await theme()).dark).toBe(true)

    await page.emulateMedia({ colorScheme: 'light' })
    await expect.poll(async () => (await theme()).dark).toBe(false)

    await openSettings(page)
    await page.getByRole('menuitemradio', { name: 'Light' }).click()
    expect((await theme()).stored).toBe('light')
    expect((await theme()).dark).toBe(false)
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect.poll(async () => (await theme()).dark).toBe(false)
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect.poll(async () => (await theme()).stored).toBe('light')
    await expect.poll(async () => (await theme()).dark).toBe(false)

    await openSettings(page)
    await page.getByRole('menuitemradio', { name: 'Dark' }).click()
    await expect.poll(async () => (await theme()).stored).toBe('dark')
    await expect.poll(async () => (await theme()).dark).toBe(true)
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect.poll(async () => (await theme()).stored).toBe('dark')
    await expect.poll(async () => (await theme()).dark).toBe(true)

    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect.poll(() => page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    const looping = await page.evaluate(() => {
      return [...document.querySelectorAll('*')].some((el) => {
        const style = getComputedStyle(el)
        const duration = parseFloat(style.animationDuration) || 0
        if (duration === 0) {
          return false
        }
        return style.animationIterationCount === 'infinite'
      })
    })
    expect(looping).toBe(false)
  } finally {
    await ui.stop()
  }
})
