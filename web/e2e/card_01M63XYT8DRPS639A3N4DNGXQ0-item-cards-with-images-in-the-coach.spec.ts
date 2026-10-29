import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

const bkb = { id: 116, key: 'black_king_bar', name: 'Black King Bar', cost: 4050, img: '/apps/dota2/images/dota_react/items/black_king_bar.png' }
const bfury = { id: 145, key: 'bfury', name: 'Battle Fury', cost: 4100, img: '/apps/dota2/images/dota_react/items/bfury.png' }

// The fixture match, its coach answering with the cards of the items the
// server found in the database for its text.
async function coachAnswers(page: Page, items: unknown[]) {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) =>
    route.fulfill(
      sse([
        ['message', { id: 1, author: 'player', kind: 'text', text: 'what do I buy?' }],
        ['decision', { category: 'items', priority: 'high', source: 'jev' }],
        [
          'answer',
          {
            kind: 'answer',
            category: 'items',
            priority: 'high',
            source: 'jev',
            textSource: 'writer',
            text: 'Finish Battle Fury, then Black King Bar against their stuns. Skip the Ethereal Blade.',
            items,
          },
        ],
        ['done', {}],
      ]),
    ),
  )
}

const chat = (page: Page) => page.getByRole('region', { name: 'Conversation' })

async function ask(page: Page) {
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('what do I buy?')
  await page.getByRole('button', { name: 'Send' }).click()
}

test('Coach answers and the build card show each suggested item as a card with its image, name and cost; an item the database lacks never gets a card', async ({
  page,
  escaped,
}) => {
  // The server sends cards only for the items the database has: Ethereal
  // Blade is in the text but has no card.
  await coachAnswers(page, [bfury, bkb])
  await page.goto('/#/coach')
  await ask(page)

  const cards = chat(page).getByRole('list', { name: 'Items in this answer' }).getByRole('listitem')
  await expect(cards).toHaveText(['Battle Fury4,100 gold', 'Black King Bar4,050 gold'])
  await expect(cards.first().locator('img')).toHaveAttribute('src', `https://cdn.cloudflare.steamstatic.com${bfury.img}`)
  await expect(chat(page)).toContainText('Skip the Ethereal Blade.')
  await expect(cards.filter({ hasText: 'Ethereal Blade' })).toHaveCount(0)

  const build = page.getByRole('region', { name: 'Build · against this lineup' })
  const situational = build.getByRole('list', { name: 'Situational items' }).getByRole('listitem')
  await expect(situational).toHaveText(['Black King Bar4,050 goldnow · vs Axe', 'Butterfly5,450 goldlater · vs Drow Ranger'])
  await expect(situational.first().locator('img')).toHaveAttribute('src', `https://cdn.cloudflare.steamstatic.com${bkb.img}`)
  await expect(build.getByRole('list', { name: 'Mid game' }).getByRole('listitem')).toHaveText(['Battle Fury4,100 gold', 'Manta Style4,650 gold'])

  await imagesLoaded(page.locator('.coach-chat'))
  await imagesLoaded(build)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('coach-item-cards.png', { fullPage: true })
})

test('Item images load from the CDN path dotaconstants gives, with a fallback when an image fails, and NOTICE credits Valve for the item art', async ({ page }) => {
  // Battle Fury's image fails; Black King Bar's loads.
  await page.route('https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/items/bfury.png', (route) => route.fulfill({ status: 404 }))
  await coachAnswers(page, [bfury, bkb])
  await page.goto('/#/coach')
  await ask(page)

  const cards = chat(page).getByRole('list', { name: 'Items in this answer' }).getByRole('listitem')
  await expect(cards.first().locator('img')).toHaveCount(0)
  await expect(cards.first().locator('.item-fallback')).toHaveText('B')
  await expect(cards.first()).toContainText('Battle Fury')
  await expect(cards.nth(1).locator('img')).toHaveAttribute('src', `https://cdn.cloudflare.steamstatic.com${bkb.img}`)
  await imagesLoaded(cards.nth(1))

  // NOTICE names the item art as Valve's and where it is loaded from.
  const fs = await import('node:fs/promises')
  const notice = await fs.readFile(path.join(fixture('..'), '..', '..', 'NOTICE'), 'utf8')
  expect(notice).toMatch(/item images[^]*The item art is Valve Corporation's/)
  await expect(chat(page).getByRole('list', { name: 'Items in this answer' })).toHaveScreenshot('item-card-fallback.png')
})
