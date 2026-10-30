import { expect, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

const bkb = { id: 116, key: 'black_king_bar', name: 'Black King Bar', cost: 4050, img: '/apps/dota2/images/dota_react/items/black_king_bar.png' }

test('An item card whose image fails to load shows the item initial in its place', async ({ page }) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) =>
    route.fulfill(
      sse([
        ['message', { id: 1, author: 'player', kind: 'text', text: 'what do I buy?' }],
        ['answer', { kind: 'answer', category: 'items', priority: 'high', textSource: 'writer', text: 'Black King Bar next.', items: [bkb] }],
        ['done', {}],
      ]),
    ),
  )
  // The CDN fails this one image.
  await page.route(`https://cdn.cloudflare.steamstatic.com${bkb.img}`, (route) => route.fulfill({ status: 404, body: '' }))
  await page.goto('/#/coach')
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('what do I buy?')
  await page.getByRole('button', { name: 'Send' }).click()
  const card = page.getByRole('region', { name: 'Conversation' }).getByRole('list', { name: 'Items in this answer' }).getByRole('listitem')
  await expect(card).toHaveText('BBlack King Bar4,050 gold')
  await expect(card.locator('img')).toHaveCount(0)
  await expect(page).toHaveScreenshot('item-fallback.png')
})
