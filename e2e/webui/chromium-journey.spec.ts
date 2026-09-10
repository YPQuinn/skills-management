import { expect, test } from '@playwright/test'
import { lstatSync, readFileSync, readlinkSync, unlinkSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { chooseSelect, expectImported, initializeStore, inspectTarget, navTo, openCreateDialog, skillLink, startUI, writeSkillFixture } from './helpers'

test.describe.configure({ mode: 'serial' })

test('Chromium WebUI core journey against embedded skillctl', async ({ page }) => {
  const ui = await startUI()
  test.setTimeout(180_000)
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    await expect(page.getByText('No Skills in local Store yet.')).toBeVisible()

    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'demo')
    await navTo(page, 'Sources')
    await openCreateDialog(page, 'Add Source')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByLabel('Name (optional)').fill('e2e-local')
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await expect(page.getByRole('heading', { name: 'e2e-local' })).toBeVisible()
    await expect(page.getByText('demo')).toBeVisible()
    await page.getByRole('button', { name: 'Import all' }).click()
    await expectImported(page)

    await navTo(page, 'Skills')
    await expect(skillLink(page, 'demo')).toBeVisible()

    await navTo(page, 'Groups')
    await openCreateDialog(page, 'Create Group')
    await page.getByLabel('Group Name').fill('crew')
    await page.getByRole('dialog').getByRole('button', { name: 'Create Group' }).click()
    await page.getByRole('list', { name: 'Managed Groups' }).getByRole('link', { name: 'crew' }).click()
    await expect(page.getByRole('heading', { name: 'crew' })).toBeVisible()
    await chooseSelect(page, 'Select a Skill to add', /demo/)
    await page.getByRole('button', { name: 'Add Skill' }).click()
    await expect(page.getByRole('table').getByRole('link', { name: 'demo', exact: true })).toBeVisible()

    const targetDir = path.join(ui.home, 'agent-skills')
    await navTo(page, 'Targets')
    await openCreateDialog(page, 'Add Distribution Target')
    await chooseSelect(page, 'Target Type', 'Custom Directory')
    await page.getByLabel('Target Name').fill('editor')
    await page.getByLabel('Directory Path').fill(targetDir)
    await page.getByRole('dialog').getByRole('button', { name: 'Add', exact: true }).click()
    await page.getByRole('table', { name: 'Registered Targets' }).getByRole('link', { name: 'editor' }).click()
    await expect(page.getByRole('heading', { name: 'editor' })).toBeVisible()
    await chooseSelect(page, 'Assignment Type', 'Skill Group')
    await chooseSelect(page, 'Skill Group', 'crew')
    await page.getByRole('button', { name: 'Assign' }).click()
    await page.getByRole('button', { name: 'Preview desired Skills' }).click()
    await expect(page.getByRole('dialog').getByRole('heading', { name: 'All Skills List' })).toBeVisible()
    await expect(page.getByRole('table', { name: 'All Skills List' }).getByRole('link', { name: 'demo', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Close' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)

    await page.getByRole('button', { name: 'Preview' }).click()
    await expect(page.getByRole('dialog').getByRole('heading', { name: 'Distribution plan' })).toBeVisible()
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await page.getByRole('button', { name: 'Distribute' }).click()
    await expect(page.getByRole('alertdialog', { name: /Distribute/ })).toBeVisible()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Distribute' }).click()
    await expect(page.getByRole('dialog', { name: /Distribution result/ })).toBeVisible()
    await expect(page.getByRole('dialog').getByText('1 created')).toBeVisible()
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    const link = path.join(targetDir, 'demo')
    const st = lstatSync(link)
    expect(st.isSymbolicLink()).toBeTruthy()
    const raw = readlinkSync(link)
    expect(path.isAbsolute(raw)).toBeTruthy()
    const storeSkill = path.join(ui.store, 'demo', 'SKILL.md')
    expect(readFileSync(storeSkill, 'utf8')).toContain('# demo')
    expect(readFileSync(path.resolve(path.dirname(link), raw, 'SKILL.md'), 'utf8')).toBe(readFileSync(storeSkill, 'utf8'))

    writeFileSync(path.join(sourceRoot, 'skills', 'demo', 'SKILL.md'), '---\nname: demo\ndescription: changed\n---\n# demo\nupdated\n')
    await navTo(page, 'Skills')
    await skillLink(page, 'demo').click()
    await page.getByRole('tab', { name: 'Synchronization' }).click()
    await expect(page).toHaveURL(/tab=synchronization/)
    const changed = page.getByText('Source changed', { exact: true })
    for (let attempt = 0; attempt < 5; attempt++) {
      const checkResponse = page.waitForResponse(response =>
        response.request().method() === 'POST' && /\/api\/v1\/skills\/\d+\/check$/.test(response.url()))
      await page.getByRole('button', { name: 'Check', exact: true }).click()
      const response = await checkResponse
      await expect(page.getByRole('button', { name: 'Check', exact: true })).toBeEnabled()
      if (response.ok()) break
      expect(await response.text()).toContain('already checking Source')
    }
    await expect(changed).toBeVisible()
    await expect(page.getByText('SKILL.md')).toBeVisible()
    await page.waitForLoadState('networkidle')
    const inSync = page.getByText('In sync', { exact: true }).first()
    for (let attempt = 0; attempt < 5; attempt++) {
      await page.getByRole('button', { name: 'Sync', exact: true }).click()
      await expect(page.getByRole('button', { name: /Syncing|Sync/ })).toBeEnabled()
      if (await inSync.isVisible()) {
        break
      }
    }
    await expect(inSync).toBeVisible()
    expect(readFileSync(storeSkill, 'utf8')).toContain('updated')
    const after = lstatSync(link)
    expect(after.isSymbolicLink()).toBeTruthy()
    expect(readlinkSync(link)).toBe(raw)
    expect(readFileSync(path.resolve(path.dirname(link), raw, 'SKILL.md'), 'utf8')).toContain('updated')

    unlinkSync(link)
    writeFileSync(link, 'unmanaged\n')
    await navTo(page, 'Targets')
    await page.getByRole('link', { name: 'editor', exact: true }).click()
    await inspectTarget(page)
    await expect(page.getByRole('cell', { name: 'Conflict' })).toBeVisible()
    await expect(page.getByRole('button', { name: /overwrite/i })).toHaveCount(0)
    expect(readFileSync(link, 'utf8')).toBe('unmanaged\n')

    await page.goto(`${ui.url}/skills/demo?tab=synchronization`)
    await expect(page.getByRole('tab', { name: 'Synchronization' })).toHaveAttribute('aria-selected', 'true')
    await page.goto(`${ui.url}/targets/editor`)
    await expect(page.getByRole('heading', { name: 'editor' })).toBeVisible()
    await page.goBack()
    await expect(page).toHaveURL(/skills\/demo/)
    await page.goForward()
    await expect(page).toHaveURL(/targets\/editor/)
  } finally {
    await ui.stop()
  }

  const restarted = await startUI(ui.home)
  try {
    await page.goto(restarted.url + '/skills/demo')
    await expect(page.getByRole('heading', { name: 'demo' })).toBeVisible()
  } finally {
    await restarted.stop()
  }
})

test('WebUI recover_store rebuilds unbound Skills', async ({ page }) => {
  const ui = await startUI()
  try {
    await page.goto(ui.url + '/setup')
    await initializeStore(page)
    const sourceRoot = path.join(ui.home, 'upstream')
    writeSkillFixture(sourceRoot, 'kept')
    await navTo(page, 'Sources')
    await openCreateDialog(page, 'Add Source')
    await page.getByLabel('Location').fill(sourceRoot)
    await page.getByRole('button', { name: 'Register and scan' }).click()
    await page.getByRole('button', { name: 'Import all' }).click()
    await expectImported(page)
  } finally {
    await ui.stop()
  }

  const stateDB = path.join(ui.home, '.skillctl', 'state.db')
  unlinkSync(stateDB)
  const again = await startUI(ui.home)
  try {
    await page.goto(again.url + '/setup')
    await expect(page.getByRole('button', { name: 'Recover Store' })).toBeVisible()
    await page.getByRole('button', { name: 'Recover Store' }).click()
    await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect(skillLink(page, 'kept')).toBeVisible()
    await skillLink(page, 'kept').click()
    await expect(page.getByText('This Skill is unbound. It was imported or created without an active Source association.', { exact: true })).toBeVisible()
  } finally {
    await again.stop()
  }
})
