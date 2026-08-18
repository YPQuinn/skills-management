import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdirSync, mkdtempSync, writeFileSync, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, type Locator, type Page } from '@playwright/test'

const here = path.dirname(fileURLToPath(import.meta.url))
export const repoRoot = path.resolve(here, '../..')
export const skillctlBin = path.join(repoRoot, 'dist', 'skillctl')

export function ensureSkillctl(): string {
  if (!existsSync(skillctlBin)) {
    throw new Error(`missing ${skillctlBin}; run ./scripts/build.sh first`)
  }
  return skillctlBin
}

export function writeSkillFixture(root: string, name: string): string {
  const dir = path.join(root, 'skills', name)
  mkdirSync(dir, { recursive: true })
  writeFileSync(
    path.join(dir, 'SKILL.md'),
    `---\nname: ${name}\ndescription: e2e fixture\n---\n# ${name}\n`,
  )
  return dir
}

export interface UIServer {
  url: string
  home: string
  store: string
  stop: () => Promise<void>
}

export async function startUI(home = mkdtempSync(path.join(tmpdir(), 'skillctl-e2e-'))): Promise<UIServer> {
  const bin = ensureSkillctl()
  const child: ChildProcessWithoutNullStreams = spawn(bin, ['ui', '--port', '0', '--no-open'], {
    env: { ...process.env, HOME: home },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let stderr = ''
  child.stderr.on('data', (chunk: Buffer) => {
    stderr += chunk.toString()
  })
  const url = await waitForURL(child, () => stderr)
  return {
    url,
    home,
    store: path.join(home, '.skillctl', 'store'),
    stop: () => stopChild(child),
  }
}

function waitForURL(child: ChildProcessWithoutNullStreams, stderr: () => string): Promise<string> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      child.kill('SIGKILL')
      reject(new Error(`skillctl ui did not print its URL\n${stderr()}`))
    }, 20_000)
    let buf = ''
    child.stdout.on('data', (chunk: Buffer) => {
      buf += chunk.toString()
      const match = buf.match(/^UI available at (\S+)/m)
      if (match) {
        clearTimeout(timer)
        resolve(match[1])
      }
    })
    child.on('exit', (code) => {
      clearTimeout(timer)
      reject(new Error(`skillctl ui exited ${code}\n${stderr()}`))
    })
  })
}

function stopChild(child: ChildProcessWithoutNullStreams): Promise<void> {
  return new Promise((resolve, reject) => {
    if (child.exitCode !== null) {
      resolve()
      return
    }
    const timer = setTimeout(() => {
      child.kill('SIGKILL')
      reject(new Error('skillctl ui did not stop after SIGTERM'))
    }, 10_000)
    child.on('exit', () => {
      clearTimeout(timer)
      resolve()
    })
    child.kill('SIGTERM')
  })
}

export async function navTo(page: Page, name: 'Skills' | 'Sources' | 'Groups' | 'Targets'): Promise<void> {
  await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name, exact: true }).click()
}

export function skillLink(page: Page, name: string): Locator {
  return page.getByRole('link', { name, exact: true })
}

export async function inspectTarget(page: Page): Promise<void> {
  for (let attempt = 0; attempt < 3; attempt++) {
    await page.getByRole('button', { name: 'Inspect' }).click()
    const locked = page.getByRole('alert').getByText(/another skillctl process/i)
    const conflict = page.getByRole('cell', { name: 'Conflict' })
    await expect(locked.or(conflict)).toBeVisible()
    if (await locked.isVisible()) {
      continue
    }
    return
  }
}

export async function expectImported(page: Page): Promise<void> {
  await expect(page.getByRole('alert').getByText(/1 imported/)).toBeVisible()
}

export async function initializeStore(page: Page): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Welcome to Skill Manager' })).toBeVisible()
  await page.getByRole('button', { name: 'Initialize' }).click()
  await expect(page.getByRole('heading', { name: 'Skills' })).toBeVisible()
}

export async function chooseSelect(page: Page, label: string, option: string): Promise<void> {
  await page.getByLabel(label).click()
  await page.getByRole('option', { name: option }).click()
}

export async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(1)
}

export async function openSettings(page: Page): Promise<Locator> {
  await page.getByRole('button', { name: 'Settings' }).click()
  return page.getByRole('menu')
}

export async function tabTo(page: Page, target: Locator, max = 80): Promise<void> {
  for (let i = 0; i < max; i++) {
    const focused = await target.evaluate((el) => {
      const active = document.activeElement
      return el === active || (active != null && el.contains(active))
    }).catch(() => false)
    if (focused) {
      return
    }
    await page.keyboard.press('Tab')
  }
  throw new Error(`could not Tab to ${await target.evaluate((el) => el.outerHTML.slice(0, 120)).catch(() => 'locator')}`)
}

export async function keyboardChooseSelect(page: Page, label: string, option: string): Promise<void> {
  const trigger = page.getByLabel(label)
  await tabTo(page, trigger)
  await page.keyboard.press('Enter')
  const item = page.getByRole('option', { name: option })
  await expect(item).toBeVisible()
  await page.keyboard.type(option)
  await expect(item).toBeFocused({ timeout: 2_000 }).catch(async () => {
    for (let i = 0; i < 20; i++) {
      const selected = await item.evaluate((el) => {
        return el.getAttribute('data-highlighted') != null
          || el.getAttribute('aria-selected') === 'true'
          || el === document.activeElement
      }).catch(() => false)
      if (selected) {
        return
      }
      await page.keyboard.press('ArrowDown')
    }
  })
  await page.keyboard.press('Enter')
}
