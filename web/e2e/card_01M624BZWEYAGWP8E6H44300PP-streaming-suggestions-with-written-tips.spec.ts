import type { Page } from '@playwright/test'
import ranking from './fixtures/suggest.json' with { type: 'json' }
import { expect, imagesLoaded, sse, test, type DraftInput } from './fixtures.ts'

const suggestions = (page: Page) => page.getByRole('region', { name: 'Suggestions' })
const items = (page: Page) => suggestions(page).getByRole('listitem')
const names = (page: Page) => items(page).locator('.suggestion-name')

const [jugg, am, bs, axe, drow] = ranking.suggestions

// Streams each draft as the server does: the code ranking, then Jev's
// order, then one tip per top hero. Position 1 gets Jev's order with written
// tips and a template (its text named a hero outside the result); any
// other position gets a decision below Jev's confidence floor.
async function streamDrafts(page: Page) {
  const drafts: DraftInput[] = []
  await page.route('**/api/draft/stream**', (route) => {
    const draft = route.request().postDataJSON() as DraftInput
    drafts.push(draft)
    const events: [string, unknown][] =
      draft.role === 1
        ? [
            ['ranking', ranking],
            ['decision', { source: 'jev', confidence: 0.62, suggestions: [bs, jugg, am, axe, drow] }],
            ['tip', { heroId: bs.heroId, source: 'writer', text: 'Bloodseeker punishes Axe early; a safe pick with a 52% patch win rate.' }],
            ['tip', { heroId: jugg.heroId, source: 'template', reason: 'rejected' }],
            ['tip', { heroId: am.heroId, source: 'writer', text: 'Anti-Mage farms safely against this lineup.' }],
            ['done', {}],
          ]
        : [
            ['ranking', ranking],
            ['decision', { source: 'ranking', reason: 'low_confidence', confidence: 0.31, suggestions: ranking.suggestions }],
            ['tip', { heroId: jugg.heroId, source: 'template', reason: 'timeout' }],
            ['tip', { heroId: am.heroId, source: 'template', reason: 'timeout' }],
            ['tip', { heroId: bs.heroId, source: 'template', reason: 'timeout' }],
            ['done', {}],
          ]
    return route.fulfill(sse(events))
  })
  return drafts
}

test('The UI updates each suggestion in place as events arrive', async ({ page, escaped }) => {
  const drafts = await streamDrafts(page)
  await page.goto('/#/draft/manual')
  // Jev reordered the ranking; each top hero carries its tip.
  await expect(names(page)).toHaveText(['Bloodseeker', 'Juggernaut', 'Anti-Mage', 'Axe', 'Drow Ranger'])
  await expect(items(page).nth(0).locator('.tip')).toHaveText('Bloodseeker punishes Axe early; a safe pick with a 52% patch win rate.')
  await expect(items(page).nth(2).locator('.tip')).toHaveText('Anti-Mage farms safely against this lineup.')
  await expect(suggestions(page).getByText('Ranked from stats')).toHaveCount(0)
  await expect(suggestions(page).getByText('Writing a tip…')).toHaveCount(0)
  await imagesLoaded(page.locator('main'))
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('stream-jev-with-tips.png', { fullPage: true })

  // A new position streams again: below the floor the code order stands, labelled.
  await page.getByRole('radio', { name: 'Position 2: Mid' }).check()
  await expect.poll(() => drafts.at(-1)?.role).toBe(2)
  await expect(names(page)).toHaveText(['Juggernaut', 'Anti-Mage', 'Bloodseeker', 'Axe', 'Drow Ranger'])
  const label = suggestions(page).getByText('Ranked from stats')
  await expect(label).toHaveAttribute('title', 'The decision model was not sure enough, so the stats order stands.')
  await expect(items(page).locator('.tip')).toHaveCount(3)
  await imagesLoaded(page.locator('main'))
  await expect(page).toHaveScreenshot('stream-low-confidence-templates.png', { fullPage: true })
})

test('A tip naming an unknown hero or item is replaced by a template', async ({ page, escaped }) => {
  await streamDrafts(page)
  await page.goto('/#/draft/manual')
  // The server replaced Juggernaut's written tip; the UI words the template from his reasons.
  await expect(items(page).nth(1).locator('.tip')).toHaveText('Juggernaut is strong against Axe; 51.2% win rate this patch.')
  await imagesLoaded(page.locator('main'))
  expect(escaped).toEqual([])
  await expect(items(page).nth(1)).toHaveScreenshot('stream-template-tip.png')
})
