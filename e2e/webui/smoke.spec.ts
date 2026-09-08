import { expect, test } from '@playwright/test'
import path from 'node:path'
import { initializeStore, navTo, openCreateDialog, startUI, writeSkillFixture } from './helpers'

test('startup, navigation, read, and one safe mutation', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await navTo(page, 'Sources')
    await expect(page.getByRole('heading', { name: 'Sources' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Add Source' })).toBeVisible()
    await navTo(page, 'Groups')
    await expect(page.getByRole('heading', { name: 'Groups' })).toBeVisible()
    await navTo(page, 'Targets')
    await expect(page.getByRole('heading', { name: 'Targets' })).toBeVisible()

    await navTo(page, 'Sources')
    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'smoke')
    await openCreateDialog(page, 'Add Source')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByLabel('Name (optional)').fill('smoke-src')
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await expect(page.getByRole('heading', { name: 'smoke-src' })).toBeVisible()
    await expect(page.getByText('skills/smoke')).toBeVisible()
  } finally {
    await ui.stop()
  }
})
