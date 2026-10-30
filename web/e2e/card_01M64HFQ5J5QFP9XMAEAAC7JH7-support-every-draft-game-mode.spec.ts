import type { Page } from '@playwright/test'
import { expect, imagesLoaded, rankFor, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

const mode = (page: Page) => page.getByRole('combobox', { name: 'Game mode' })

async function pick(page: Page, row: string, slot: string, dialog: string, search: string, hero: string) {
  await page.getByRole('group', { name: row }).getByRole('button', { name: slot }).first().click()
  const picker = page.getByRole('dialog', { name: dialog })
  await picker.getByRole('searchbox').fill(search)
  await picker.getByRole('button', { name: hero, exact: true }).click()
  await expect(picker).toBeHidden()
}

test('The draft screen offers the mode (default All Pick, remembered per user); a match session stores it, and Ability Draft or a custom game shows "not supported"', async ({
  page,
  drafts,
}) => {
  await page.goto('/#/draft/manual')
  await expect(mode(page)).toHaveValue('all_pick_ranked')
  await expect(mode(page).locator('option[value="ability_draft"]')).toHaveAttribute('disabled', '')
  await expect(mode(page).locator('option[value="ability_draft"]')).toHaveText('Ability Draft (not supported)')
  await expect(mode(page).locator('option[value="custom"]')).toHaveText('Custom games (not supported)')
  await expect(page.getByText('Ability Draft and custom games are not supported')).toBeVisible()

  await mode(page).selectOption('turbo')
  await expect.poll(() => drafts.at(-1)?.mode).toBe('turbo')
  // Remembered: a reload keeps Turbo.
  await page.reload()
  await expect(mode(page)).toHaveValue('turbo')

  // The match started from the draft carries the mode.
  const started: unknown[] = []
  await page.route('**/api/match', (route) => {
    started.push(route.request().postDataJSON())
    return route.fulfill({ json: { ...session, mode: 'turbo', messages: [] } })
  })
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, mode: 'turbo', messages: [] } }))
  await page.getByRole('button', { name: /^Pick .* and start$/ }).first().click()
  await expect.poll(() => (started[0] as { mode?: string } | undefined)?.mode).toBe('turbo')
  // The started match opens the Coach tab before the draft is opened again.
  await expect(page).toHaveURL(/#\/coach$/)
  await page.goto('/#/draft/manual')
  await expect(mode(page)).toBeVisible()
  await expect(page).toHaveScreenshot('mode-picker.png')
})

test('Captains Mode suggests bans and picks; Single and Random Draft rank only the offered heroes; All Random skips suggestions and opens the build (tests per mode with fixed data)', async ({
  page,
  drafts,
}) => {
  // The server's bans for Captains Mode, from fixed data.
  await page.route('**/api/draft/stream**', (route) => {
    const d = route.request().postDataJSON() as { allies: number[]; enemies: number[]; bans: number[]; role: number; mode: string; pool?: number[] }
    drafts.push(d)
    let suggestions = rankFor(d)
    if (d.pool) suggestions = suggestions.filter((s) => d.pool!.includes(s.heroId))
    const bans = d.mode === 'captains_mode' ? suggestions.slice(0, 2).map((s) => ({ ...s, counter: 4.2 })) : undefined
    return route.fulfill(
      sse([
        ['ranking', { patch: '7.41', suggestions, bans, mode: d.mode, stats: 'ranked' }],
        ['decision', { source: 'jev', confidence: 0.4, suggestions }],
        ['done', {}],
      ]),
    )
  })
  await page.goto('/#/draft/manual')

  await mode(page).selectOption('captains_mode')
  const bans = page.getByRole('list', { name: 'Ban suggestions' }).getByRole('listitem')
  await expect(bans).toHaveCount(2)
  await expect(page.getByRole('list', { name: 'Suggestions' }).getByRole('listitem').first()).toBeVisible()

  await mode(page).selectOption('single_draft')
  await expect(page.getByRole('list', { name: 'Ban suggestions' })).toHaveCount(0)
  await expect(page.getByText('Tap the heroes the game offers you; only they are ranked.')).toBeVisible()
  await pick(page, 'Heroes offered', 'Add ally', 'Heroes offered', 'jugg', 'Juggernaut')
  await pick(page, 'Heroes offered', 'Add ally', 'Heroes offered', 'axe', 'Axe')
  await expect.poll(() => drafts.at(-1)?.pool).toEqual([8, 2])
  await expect(page.getByRole('list', { name: 'Suggestions' }).locator('.suggestion-name')).toHaveText(['Juggernaut', 'Axe'])

  await mode(page).selectOption('all_random')
  const sent = drafts.length
  await expect(page.getByText('All Random picks your hero for you.')).toBeVisible()
  await pick(page, 'Your hero', 'Add ally', 'Your hero', 'jugg', 'Juggernaut')
  await expect(page.getByRole('button', { name: 'Pick Juggernaut and start' })).toBeVisible()
  expect(drafts.length).toBe(sent)
  await imagesLoaded(page.locator('.board'))
  await expect(page).toHaveScreenshot('mode-all-random.png')
})

test('Turbo builds and coach triggers use Turbo data or Turbo-scaled thresholds, and show which one they use', async ({ page }) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route('**/api/draft/stream**', (route) => {
    const d = route.request().postDataJSON() as { allies: number[]; enemies: number[]; bans: number[]; role: number }
    return route.fulfill(sse([['ranking', { patch: '7.41', suggestions: rankFor(d), mode: 'turbo', stats: 'ranked' }], ['done', {}]]))
  })
  await page.goto('/#/draft/manual')
  await mode(page).selectOption('turbo')
  await expect(page.getByText('No Turbo stats: ranked stats')).toBeVisible()

  // The Turbo match's build says it reads the general item data.
  await page.route(`**/api/match/${session.id}`, (route) =>
    route.fulfill({ json: { ...session, mode: 'turbo', itemPlan: { ...session.itemPlan, mode: 'turbo', data: 'general' }, messages: [] } }),
  )
  await page.goto('/#/coach')
  await expect(page.getByRole('region', { name: 'Build · against this lineup' }).getByText('General item data (not Turbo)')).toBeVisible()
})

test('The readers flag slots on a layout they are not calibrated for; e2e covers the mode picker on phone and desktop', async ({ page }) => {
  // Captains Mode is remembered; the read comes back on an uncalibrated
  // layout with every slot flagged.
  await page.addInitScript(() => window.localStorage.setItem('draftmind.mode', 'captains_mode'))
  const urls: string[] = []
  const unknown = { status: 'unknown', confidence: 0 }
  await page.route('**/api/draft/stream**', (route) => {
    urls.push(route.request().url())
    return route.fulfill(
      sse([
        ['read', { patch: '7.41', reader: 'jev', layout: 'uncalibrated', radiant: Array(5).fill(unknown), dire: Array(5).fill(unknown), draft: { allies: [], enemies: [], bans: [], role: 1 } }],
        ['ranking', { patch: '7.41', suggestions: rankFor({ allies: [], enemies: [], bans: [], role: 1 }), mode: 'captains_mode', stats: 'ranked' }],
        ['done', {}],
      ]),
    )
  })
  await page.goto('/#/draft')
  await page.locator('input[type=file]').first().setInputFiles('e2e/fixtures/pick-photo.jpg')
  await expect(page.getByText("This mode's pick screen is not calibrated: tap each hero.")).toBeVisible()
  await expect(page.getByRole('button', { name: 'Unread hero, choose' })).toHaveCount(9)
  expect(new URL(urls[0]).searchParams.get('mode')).toBe('captains_mode')
  await expect(page.getByRole('region', { name: 'Draft read from the photo' })).toHaveScreenshot('mode-uncalibrated-read.png')
})
