import { expect, portraitsLoaded, test } from './fixtures.ts'

// Literal sizes from the card, keyed by project.
const sizes: Record<string, { width: number; height: number }> = {
  phone: { width: 390, height: 844 },
  desktop: { width: 1280, height: 800 },
}

test('runs at the project size against the mocked API', async ({ page, escaped }, testInfo) => {
  await page.goto('/#/heroes')

  await expect(page.getByRole('status').filter({ hasText: 'API:' })).toHaveText('API: ok, database ok')
  await expect(page.getByText('Patch 7.41')).toBeVisible()
  expect(await page.evaluate(() => ({ width: innerWidth, height: innerHeight }))).toEqual(sizes[testInfo.project.name])
  expect(escaped).toEqual([])
})

test('the hero grid matches its baseline', async ({ page }) => {
  await page.goto('/#/heroes')

  const grid = page.getByRole('list', { name: 'Heroes' })
  await expect(grid.getByRole('listitem')).toHaveCount(8)
  await portraitsLoaded(page)
  await expect(page).toHaveScreenshot('hero-grid.png', { fullPage: true })
})
