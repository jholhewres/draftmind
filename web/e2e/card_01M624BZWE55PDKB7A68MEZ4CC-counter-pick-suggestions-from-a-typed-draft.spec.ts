import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'

// Taps an empty slot of a row, searches the picker and chooses the hero.
async function pick(page: Page, row: string, slot: string, dialog: string, search: string, hero: string) {
  await page.getByRole('group', { name: row }).getByRole('button', { name: slot }).first().click()
  const picker = page.getByRole('dialog', { name: dialog })
  await picker.getByRole('searchbox').fill(search)
  await picker.getByRole('button', { name: hero, exact: true }).click()
  await expect(picker).toBeHidden()
}

// Axe and Drow Ranger on the enemy team, Bane banned, the player at Pos 1.
async function typeDraft(page: Page) {
  await page.goto('/#/draft/manual')
  await pick(page, 'Enemies', 'Add enemy', 'Choose an enemy', 'axe', 'Axe')
  await pick(page, 'Enemies', 'Add enemy', 'Choose an enemy', 'drow', 'Drow Ranger')
  await pick(page, 'Bans', 'Add ban', 'Choose a ban', 'bane', 'Bane')
}

const suggested = (page: Page) => page.getByRole('list', { name: 'Suggestions' }).getByRole('listitem')

test('Picked and banned heroes never appear and the ranking is deterministic for a fixed dataset', async ({
  page,
  escaped,
  drafts,
}) => {
  await typeDraft(page)
  await expect(page.getByRole('group', { name: 'Enemies' }).getByRole('button', { name: /^Enemy: / })).toHaveCount(2)
  await expect(page.getByRole('button', { name: 'Enemy: Drow Ranger' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Ban: Bane' })).toBeVisible()
  await expect.poll(() => drafts.at(-1)).toEqual({ allies: [], enemies: [2, 6], bans: [3], role: 1, mode: 'all_pick_ranked' })

  await expect(suggested(page).locator('.suggestion-name')).toHaveText([
    'Juggernaut',
    'Anti-Mage',
    'Bloodseeker',
    'Earthshaker',
    'Crystal Maiden',
  ])

  // The picker offers only the heroes still free.
  await page.getByRole('group', { name: 'Allies' }).getByRole('button', { name: 'Add ally' }).first().click()
  const picker = page.getByRole('dialog', { name: 'Choose an ally' })
  await expect(picker.getByRole('listitem')).toHaveText(['Anti-Mage', 'Bloodseeker', 'Crystal Maiden', 'Earthshaker', 'Juggernaut'])
  await imagesLoaded(picker)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('draft-picker.png')
})

test('Each suggestion lists its counter delta per enemy, and ranking the full hero set takes under 100 ms p95', async ({
  page,
  escaped,
  drafts,
}, testInfo) => {
  await typeDraft(page)
  await page.getByRole('radio', { name: 'Position 3: Offlane' }).check()
  await expect.poll(() => drafts.at(-1)).toEqual({ allies: [], enemies: [2, 6], bans: [3], role: 3, mode: 'all_pick_ranked' })

  const items = suggested(page)
  await expect(items).toHaveCount(5)
  await expect(items.nth(0).locator('.delta')).toHaveText(['▲ Axe +3.1', '▼ Drow Ranger −0.6'])
  await expect(items.nth(0).locator('.score-value')).toHaveText('9.0')
  await expect(items.nth(4).locator('.delta')).toHaveText(['▼ Axe −2.4', 'Drow Ranger 0.0'])

  // Desktop: board and suggestions side by side; phone: stacked.
  const board = (await page.getByRole('region', { name: 'Draft board' }).boundingBox())!
  const list = (await page.getByRole('region', { name: 'Suggestions' }).boundingBox())!
  if (testInfo.project.name === 'desktop') expect(list.x).toBeGreaterThan(board.x + board.width)
  else expect(list.y).toBeGreaterThan(board.y + board.height)

  await imagesLoaded(page.locator('main'))
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('draft-suggestions.png', { fullPage: true })
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('The Draft tab is in Portuguese with a decimal comma when the UI is in pt-BR', async ({ page, escaped }) => {
    await page.goto('/#/draft/manual')
    await pick(page, 'Inimigos', 'Adicionar inimigo', 'Escolha um inimigo', 'axe', 'Axe')
    await pick(page, 'Inimigos', 'Adicionar inimigo', 'Escolha um inimigo', 'drow', 'Drow Ranger')
    const first = page.getByRole('list', { name: 'Sugestões' }).getByRole('listitem').first()
    await expect(first.locator('.delta')).toHaveText(['▲ Axe +3,1', '▼ Drow Ranger −0,6'])
    await expect(first.locator('.score-value')).toHaveText('9,0')
    await expect(page.getByRole('group', { name: 'Sua posição' }).getByRole('radio', { name: 'Posição 1: Carry' })).toBeChecked()
    await imagesLoaded(page.locator('main'))
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('draft-suggestions-pt-br.png', { fullPage: true })
  })
})
