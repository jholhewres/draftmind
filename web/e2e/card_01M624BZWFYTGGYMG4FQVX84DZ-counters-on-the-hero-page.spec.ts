import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

// Axe's page and its counters on 7.41; Axe has no history synced.
async function mockAxe(page: Page) {
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      const lang = url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en'
      if (url.pathname === '/api/heroes/axe/counters') return route.fulfill({ path: fixture('counters-axe.json') })
      if (url.pathname === '/api/heroes/axe') return route.fulfill({ path: fixture(`hero-axe.${lang}.json`) })
      return route.fulfill({ status: 404, json: { error: 'no such hero' } })
    },
  )
}

test("Top counters and victims come from the current patch's hero_matchup rows", async ({ page, escaped }) => {
  await mockAxe(page)
  await page.goto('/#/heroes/axe')

  const counters = page.getByRole('region', { name: 'Counters · patch 7.41' })
  const good = counters.getByRole('list', { name: 'Good against' })
  const bad = counters.getByRole('list', { name: 'Bad against' })
  await expect(good.getByRole('listitem')).toHaveText([/Anti-Mage\s*\+3\.1/, /Crystal Maiden\s*\+2\.8/, /Bloodseeker\s*\+1\.2/])
  await expect(bad.getByRole('listitem')).toHaveText([/Juggernaut\s*[-−]2\.4/, /Earthshaker\s*[-−]1\.8/, /Drow Ranger\s*[-−]0\.9/])
  await expect(good.getByRole('link', { name: /Anti-Mage/ })).toHaveAttribute('href', '#/heroes/antimage')

  await imagesLoaded(counters)
  expect(escaped).toEqual([])
  await expect(counters).toHaveScreenshot('hero-counters.png')
})

test('Low-sample matchups are marked as weaker', async ({ page }) => {
  await mockAxe(page)
  await page.goto('/#/heroes/axe')

  const counters = page.getByRole('region', { name: 'Counters · patch 7.41' })
  await expect(counters.locator('.low-sample')).toHaveCount(2)
  await expect(counters.getByRole('listitem').filter({ hasText: 'Bloodseeker' })).toHaveClass('low-sample')
  await expect(counters.getByRole('listitem').filter({ hasText: 'Bloodseeker' })).toContainText('few games')
  await expect(counters.getByRole('listitem').filter({ hasText: 'Anti-Mage' })).not.toHaveClass('low-sample')
  await expect(counters.getByText('Faded: under 100 games on this patch, a weaker sign.')).toBeVisible()
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('Every number on the page comes from current patch tables', async ({ page, escaped }) => {
    await mockAxe(page)
    await page.goto('/#/heroes/axe')

    // The patch the counters were read on, and its deltas in the reader's locale.
    const counters = page.getByRole('region', { name: 'Counters · patch 7.41' })
    await expect(counters.getByRole('list', { name: 'Bom contra' }).getByRole('listitem').first()).toHaveText(/Anti-Mage\s*\+3,1/)
    await expect(counters.getByRole('list', { name: 'Sofre contra' }).getByRole('listitem').first()).toHaveText(/Juggernaut\s*[-−]2,4/)
    await expect(counters.getByRole('listitem').filter({ hasText: 'Anti-Mage' })).toHaveAttribute('title', '412 jogos')

    await imagesLoaded(page.locator('.hero-page'))
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('hero-page-counters-pt-br.png', { fullPage: true })
  })
})
