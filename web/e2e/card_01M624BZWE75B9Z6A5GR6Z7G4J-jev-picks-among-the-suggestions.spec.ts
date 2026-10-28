import type { Page } from '@playwright/test'
import ranking from './fixtures/suggest.json' with { type: 'json' }
import { expect, imagesLoaded, test } from './fixtures.ts'

const suggestions = (page: Page) => page.getByRole('region', { name: 'Suggestions' })
const names = (page: Page) => suggestions(page).getByRole('listitem').locator('.suggestion-name')

// Answers POST /api/draft/suggest with the fixture ranking in the given
// order and source; registered after the shared mock, so it wins.
async function answer(page: Page, body: object) {
  await page.route('**/api/draft/suggest', (route) => route.fulfill({ json: { patch: ranking.patch, ...body } }))
}

test('A timeout falls back to the code ranking with a label', async ({ page, escaped }) => {
  await answer(page, { source: 'ranking', reason: 'timeout', suggestions: ranking.suggestions })
  await page.goto('/#/draft')
  const label = suggestions(page).getByText('Ranked from stats')
  await expect(label).toBeVisible()
  await expect(label).toHaveAttribute('title', 'The decision model did not answer in time.')
  await expect(names(page)).toHaveText(['Juggernaut', 'Anti-Mage', 'Bloodseeker', 'Axe', 'Drow Ranger'])
  await imagesLoaded(page.locator('main'))
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('draft-ranking-fallback.png', { fullPage: true })
})

test("Jev's order shows without the fallback label", async ({ page, escaped }) => {
  const [jugg, am, bs, ...rest] = ranking.suggestions
  await answer(page, { source: 'jev', confidence: 0.35, suggestions: [bs, jugg, am, ...rest] })
  await page.goto('/#/draft')
  await expect(names(page)).toHaveText(['Bloodseeker', 'Juggernaut', 'Anti-Mage', 'Axe', 'Drow Ranger'])
  await expect(suggestions(page).getByText('Ranked from stats')).toHaveCount(0)
  await imagesLoaded(page.locator('main'))
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('draft-jev-order.png', { fullPage: true })
})
