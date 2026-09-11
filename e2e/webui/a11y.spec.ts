import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import path from 'node:path'
import { expectImported, initializeStore, navTo, openCreateDialog, skillLink, startUI, writeSkillFixture } from './helpers'

async function expectAxeClean(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).analyze()
  const serious = results.violations.filter((v) => v.impact === 'critical' || v.impact === 'serious')
  expect(serious, JSON.stringify(serious, null, 2)).toEqual([])
}

test('axe has no critical or serious violations on setup and resource pages', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await expect(page.getByRole('heading', { name: 'Welcome to Skill Manager' })).toBeVisible()
    await expectAxeClean(page)

    await initializeStore(page)
    await expectAxeClean(page)

    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'a11y')
    await navTo(page, 'Sources')
    await expectAxeClean(page)
    await openCreateDialog(page, 'Add Source')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await expect(page.getByRole('heading', { name: 'upstream' })).toBeVisible()
    await expect(page.getByText('skills/a11y')).toBeVisible()
    await page.getByRole('button', { name: 'Import all' }).click()
    await expectImported(page)
    await expectAxeClean(page)

    await navTo(page, 'Skills')
    await skillLink(page, 'a11y').click()
    await expect(page.getByRole('heading', { name: 'a11y', level: 1 })).toBeVisible()
    await expectAxeClean(page)

    await navTo(page, 'Groups')
    await expectAxeClean(page)
    await navTo(page, 'Targets')
    await expectAxeClean(page)
  } finally {
    await ui.stop()
  }
})
