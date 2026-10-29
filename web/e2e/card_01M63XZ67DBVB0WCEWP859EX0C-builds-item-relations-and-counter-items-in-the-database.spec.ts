import path from 'node:path'
import { expect, imagesLoaded, test } from './fixtures.ts'

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

test('The hero page shows its builds per position with item images, cost and source, on phone and desktop', async ({ page, escaped }) => {
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      if (url.pathname === '/api/heroes/axe/build') return route.fulfill({ path: fixture('build-axe.json') })
      if (url.pathname === '/api/heroes/axe') return route.fulfill({ path: fixture('hero-axe.en.json') })
      return route.fulfill({ status: 404, json: { error: 'no such hero' } })
    },
  )
  await page.goto('/#/heroes/axe')

  const builds = page.getByRole('region', { name: 'Builds by position' })
  const offlane = builds.getByRole('article', { name: 'Offlane' })
  await expect(offlane.getByRole('heading', { level: 3 })).toHaveText('OfflaneFrom OpenDota')
  await expect(offlane.getByText('182,400 games · 51.3% wins')).toBeVisible()
  await expect(offlane.getByRole('list', { name: 'Mid game' }).getByRole('listitem')).toHaveText(['Blink Dagger2,250 gold', 'Blade Mail2,100 gold'])
  await expect(offlane.getByRole('listitem').first().locator('img')).toHaveAttribute(
    'src',
    'https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/items/tango.png',
  )
  const support = builds.getByRole('article', { name: 'Support' })
  await expect(support.getByRole('heading', { level: 3 })).toHaveText('SupportCurated')
  await expect(support.getByRole('list', { name: 'Starting' }).getByRole('listitem')).toHaveCount(3)

  await imagesLoaded(builds)
  expect(escaped).toEqual([])
  await expect(builds).toHaveScreenshot('hero-builds.png')
})

test('The coach\'s build and item advice cites only builds and items from these tables (test: an answer naming an item outside them gets the template)', async ({
  page,
}) => {
  // The "Items now?" chip is answered by the server's code from the stored
  // plan, with no model call; a writer answer the server rejected arrives
  // with no text and is worded by the template.
  const { default: session } = await import('./fixtures/match.json', { with: { type: 'json' } })
  const bkb = session.itemPlan.situational[0].item
  const bodies: unknown[] = []
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const body = route.request().postDataJSON() as { text: string; quick?: string }
    bodies.push(body)
    const answer = body.quick
      ? { kind: 'answer', category: 'items', priority: 'high', source: 'code', textSource: 'template', reason: 'quick', items: [bkb] }
      : { kind: 'answer', category: 'items', priority: 'high', source: 'jev', textSource: 'template', reason: 'rejected', text: 'Buy a Divine Rapier.' }
    const events = [
      ['decision', { category: 'items', priority: 'high', source: answer.source }],
      ['answer', answer],
      ['done', {}],
    ]
    return route.fulfill({ contentType: 'text/event-stream', body: events.map(([n, d]) => `event: ${n}\ndata: ${JSON.stringify(d)}\n\n`).join('') })
  })
  await page.goto('/#/coach')
  const chat = page.getByRole('region', { name: 'Conversation' })

  await page.getByRole('group', { name: 'Quick questions' }).getByRole('button', { name: 'Items now?' }).click()
  const reply = chat.getByRole('listitem').filter({ hasText: 'Buy these next' })
  await expect(reply).toContainText('Buy these next, most urgent first:')
  await expect(reply.getByRole('list', { name: 'Items in this answer' }).getByRole('listitem')).toHaveText(['Black King Bar4,050 gold'])

  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('what about rapier?')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(chat.getByText('No written advice this time. Check your build above and ask again.')).toBeVisible()
  await expect(chat).not.toContainText('Divine Rapier')
  expect(bodies).toEqual([{ text: 'Items now?', quick: 'items' }, { text: 'what about rapier?' }])
  await expect(chat).toHaveScreenshot('coach-quick-items.png')
})
