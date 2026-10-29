import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

// Taps an empty slot of a row, searches the picker and chooses the hero.
async function pick(page: Page, row: string, slot: string, dialog: string, search: string, hero: string) {
  await page.getByRole('group', { name: row }).getByRole('button', { name: slot }).first().click()
  const picker = page.getByRole('dialog', { name: dialog })
  await picker.getByRole('searchbox').fill(search)
  await picker.getByRole('button', { name: hero, exact: true }).click()
  await expect(picker).toBeHidden()
}

type Turn = [string, unknown][]

// A match server: POST /api/match starts the fixture session, GET returns
// it with every message stored so far, and each message gets the next
// scripted coach turn.
async function matchServer(page: Page, turns: Turn[]) {
  const stored: unknown[] = []
  const started: unknown[] = []
  await page.route('**/api/match', (route) => {
    started.push(route.request().postDataJSON())
    return route.fulfill({ path: fixture('match.json') })
  })
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: stored } }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const { text } = route.request().postDataJSON() as { text: string }
    const mine = { id: stored.length + 1, author: 'player', kind: 'text', text }
    stored.push(mine)
    const turn = turns.shift()!
    for (const [name, data] of turn) {
      if (name === 'answer' || name === 'followup') {
        const d = data as Record<string, string>
        stored.push({ id: stored.length + 1, author: 'coach', kind: name, text: d.textSource === 'writer' ? d.text : '', category: d.category, priority: d.priority, reason: d.reason })
      }
    }
    return route.fulfill(sse([['message', mine], ...turn, ['done', {}]]))
  })
  return started
}

const answer = (text: string, textSource = 'writer', reason?: string): Turn => [
  ['situation', { category: 'lane', summary: 'Axe is pressuring the lane.', heroes: [2], abilities: [], items: [], unknown: [] }],
  ['decision', { category: 'lane', priority: 'high', source: 'jev' }],
  ['answer', { kind: 'answer', category: 'lane', priority: 'high', source: 'jev', text: textSource === 'writer' ? text : undefined, textSource, reason }],
]

const chat = (page: Page) => page.getByRole('region', { name: 'Conversation' })

async function send(page: Page, text: string) {
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill(text)
  await page.getByRole('button', { name: 'Send' }).click()
}

test('The match session survives a page reload', async ({ page, escaped, drafts }) => {
  const started = await matchServer(page, [answer('Stay behind your creeps: Axe wins trades early. Buy Black King Bar before you fight.')])
  await page.goto('/#/draft/manual')
  await pick(page, 'Enemies', 'Add enemy', 'Choose an enemy', 'axe', 'Axe')
  await pick(page, 'Enemies', 'Add enemy', 'Choose an enemy', 'drow', 'Drow Ranger')
  await pick(page, 'Bans', 'Add ban', 'Choose a ban', 'bane', 'Bane')
  await expect.poll(() => drafts.at(-1)).toEqual({ allies: [], enemies: [2, 6], bans: [3], role: 1 })

  await page.getByRole('button', { name: 'Pick Juggernaut and start' }).click()
  await expect(page).toHaveURL(/#\/coach$/)
  expect(started).toEqual([{ hero: 8, allies: [], enemies: [2, 6], bans: [3], role: 1 }])
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Juggernaut · Carry')
  await expect(page.getByRole('list', { name: 'Situational items' }).getByRole('listitem')).toHaveText([
    'Black King Bar now · vs Axe',
    'Butterfly later · vs Drow Ranger',
  ])

  await send(page, 'axe keeps calling me in lane')
  await expect(chat(page).getByRole('listitem')).toHaveText([
    'axe keeps calling me in lane',
    /Lane · high priority\s*Stay behind your creeps/,
  ])

  await page.reload()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Juggernaut · Carry')
  await expect(chat(page).getByRole('listitem')).toHaveText([
    'axe keeps calling me in lane',
    /Lane · high priority\s*Stay behind your creeps/,
  ])

  await imagesLoaded(page.locator('.coach-head'))
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('coach-chat.png', { fullPage: true })
})

test('Answers name only heroes, abilities and items in the database for the current patch', async ({ page }) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await matchServer(page, [answer('', 'template', 'rejected')])
  await page.goto('/#/coach')

  // The server's entity check rejected the written answer: the template stands.
  await send(page, 'what should I buy vs axe')
  const reply = chat(page).getByRole('listitem').nth(1)
  await expect(reply).toHaveText(/Lane · high priority\s*No written advice this time\. Check your build above and ask again\./)
})

test('A message that cannot be parsed gets a short follow-up question instead of a guess', async ({ page }) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await matchServer(page, [[['followup', { kind: 'followup', reason: 'unclear' }]]])
  await page.goto('/#/coach')

  await send(page, 'hm')
  await expect(chat(page).getByRole('listitem')).toHaveText([
    'hm',
    'Tell me a bit more: is it the lane, a fight, items or an objective, and which heroes are involved?',
  ])
  await expect(chat(page).locator('.advice-tag')).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Conversation' })).toHaveScreenshot('coach-followup.png')
})
