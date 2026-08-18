import { expect, test } from '@playwright/test'
import path from 'node:path'
import { expectImported, expectNoHorizontalOverflow, initializeStore, navTo, openSettings, startUI, writeSkillFixture } from './helpers'

test('desktop master-detail, narrow list/detail split, no overflow', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'wide')
    await navTo(page, 'Sources')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await page.getByRole('button', { name: 'Import all' }).click()
    await expectImported(page)

    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto(`${ui.url}/skills/wide`)
    await expect(page.getByRole('heading', { name: 'wide' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toBeVisible()
    await expectNoHorizontalOverflow(page)

    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(`${ui.url}/skills`)
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toHaveCount(0)
    await expectNoHorizontalOverflow(page)
    await page.getByRole('link', { name: 'wide', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'wide' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'All Skills' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Skill list' })).toHaveCount(0)
    await expectNoHorizontalOverflow(page)
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
