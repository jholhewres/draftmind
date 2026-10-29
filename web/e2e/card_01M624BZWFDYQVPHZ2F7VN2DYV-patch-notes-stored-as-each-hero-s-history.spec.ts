import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)
const langOf = (url: URL) => (url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en')

// The patches 7.41f and 7.41e and Axe's history as the API serves them,
// built from the captured feed fixtures. Records each patch asked for.
async function mockPatches(page: Page) {
  const asked: string[] = []
  await page.route('**/api/patches', (route) => route.fulfill({ path: fixture('patches.json') }))
  await page.route('**/api/patches/*', (route) => {
    const url = new URL(route.request().url())
    const name = decodeURIComponent(url.pathname.split('/').at(-1)!)
    asked.push(`${name}:${langOf(url)}`)
    return route.fulfill({ path: fixture(`patch-${name}.${langOf(url)}.json`) })
  })
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      if (url.pathname === '/api/heroes/axe/history') return route.fulfill({ path: fixture(`history-axe.${langOf(url)}.json`) })
      if (url.pathname === '/api/heroes/axe') return route.fulfill({ path: fixture(`hero-axe.${langOf(url)}.json`) })
      return route.fulfill({ status: 404, json: { error: 'no such hero' } })
    },
  )
  return asked
}

test("The Patches tab shows a patch's hero and item changes and filters them by hero", async ({ page, escaped }) => {
  const asked = await mockPatches(page)
  await page.goto('/#/patches')

  const picker = page.getByRole('group', { name: 'Choose a patch' })
  await expect(picker.getByRole('button', { name: '7.41f · latest' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByText('35 heroes and 17 items changed · released Sep 15, 2026')).toBeVisible()
  const heroes = page.getByRole('list', { name: 'Heroes' })
  await expect(heroes.getByRole('heading', { level: 3 })).toHaveText(['Anti-Mage', 'Bane'])
  await expect(heroes.getByText('Mana Burned as Damage increased from 60% to 65%')).toBeVisible()
  await expect(heroes.getByRole('link', { name: 'Bane' })).toHaveAttribute('href', '#/heroes/bane')
  const items = page.getByRole('list', { name: 'Items' })
  await expect(items.getByRole('heading', { level: 3 })).toHaveText(['Battle Fury'])
  await expect(items.getByText('Chop Tree cooldown decreased from 4s to 3s')).toBeVisible()

  // Another patch, then one hero: the filter keeps its hero across patches.
  await picker.getByRole('button', { name: '7.41e' }).click()
  await expect(heroes.getByRole('heading', { level: 3 })).toHaveText(['Axe', 'Bane', 'Drow Ranger'])
  await page.getByLabel('Hero', { exact: true }).selectOption({ label: 'Bane' })
  await expect(heroes.getByRole('heading', { level: 3 })).toHaveText(['Bane'])
  await expect(heroes.getByText('Max Terrors per hero increased from 5 to 6')).toBeVisible()
  await expect(page.getByRole('list', { name: 'Items' })).toHaveCount(0)
  await page.getByLabel('Hero', { exact: true }).selectOption({ label: 'Juggernaut' })
  await expect(page.getByText('No changes to Juggernaut in this patch.')).toBeVisible()
  await page.getByLabel('Hero', { exact: true }).selectOption({ label: 'Bane' })
  await picker.getByRole('button', { name: '7.41f · latest' }).click()
  await expect(heroes.getByRole('heading', { level: 3 })).toHaveText(['Bane'])
  await expect(heroes.getByText('Base Armor decreased by 1')).toBeVisible()
  expect(new Set(asked)).toEqual(new Set(['7.41f:en', '7.41e:en']))

  await imagesLoaded(heroes)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('patches-filtered.png', { fullPage: true })
})

test("GET /api/heroes/{slug}/history returns the hero's notes, newest patch first", async ({ page, escaped }) => {
  await mockPatches(page)
  await page.goto('/#/heroes/axe')

  const history = page.getByRole('region', { name: 'History' })
  await expect(history.locator('.history-patch')).toHaveText(['7.41e', '7.41'])
  await expect(history.getByText('Battle Hunger')).toBeVisible()
  await expect(history.getByText('Damage per second decreased from 12/18/24/30 to 12/16/20/24')).toBeVisible()

  await history.scrollIntoViewIfNeeded()
  expect(escaped).toEqual([])
  await expect(history).toHaveScreenshot('hero-history.png')
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('The Patches tab and the hero history read in Portuguese when the UI is in pt-BR', async ({ page, escaped }) => {
    const asked = await mockPatches(page)
    await page.goto('/#/patches')

    await expect(page.getByRole('button', { name: '7.41f · atual' })).toHaveAttribute('aria-pressed', 'true')
    const heroes = page.getByRole('list', { name: 'Heróis' })
    await expect(heroes.getByText('Redução da armadura base em 1')).toBeVisible()
    expect(asked).toContain('7.41f:pt-BR')
    await imagesLoaded(heroes)
    await expect(page).toHaveScreenshot('patches-pt-br.png', { fullPage: true })

    await page.goto('/#/heroes/axe')
    await expect(page.getByRole('region', { name: 'Histórico' }).locator('.history-patch')).toHaveText(['7.41e', '7.41'])
    expect(escaped).toEqual([])
  })
})
