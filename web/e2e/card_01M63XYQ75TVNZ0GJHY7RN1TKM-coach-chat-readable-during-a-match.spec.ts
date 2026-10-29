import type { Page, Route } from '@playwright/test'
import { expect, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

type Reply = (route: Route, text: string) => Promise<void>

const answer = (text: string): [string, unknown][] => [
  ['message', { id: 100, author: 'player', kind: 'text', text }],
  ['decision', { category: 'lane', priority: 'high', source: 'jev' }],
  ['answer', { kind: 'answer', category: 'lane', priority: 'high', source: 'jev', textSource: 'writer', text: `Coach on “${text}”: stay behind your creeps until level 6.` }],
  ['done', {}],
]

const ok: Reply = (route, text) => route.fulfill(sse(answer(text)))

// The fixture match, restored with past messages; each POST is answered by
// the next reply in order (the last one repeats). Returns the texts sent.
async function coachServer(page: Page, replies: Reply[], past = 0) {
  const messages = Array.from({ length: past }, (_, i) =>
    i % 2 === 0
      ? { id: i + 1, author: 'player', kind: 'text', text: `Question ${i / 2 + 1} about the lane` }
      : { id: i + 1, author: 'coach', kind: 'answer', category: 'items', priority: 'medium', text: `Answer ${(i + 1) / 2}: buy Phase Boots, then Battle Fury.` },
  )
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages } }))
  const sent: string[] = []
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const { text } = route.request().postDataJSON() as { text: string }
    sent.push(text)
    const reply = replies[Math.min(sent.length - 1, replies.length - 1)]
    return reply(route, text)
  })
  return sent
}

const chat = (page: Page, name = 'Conversation') => page.getByRole('region', { name })
const bubbles = (page: Page, name?: string) => chat(page, name).getByRole('list').getByRole('listitem')

async function send(page: Page, text: string) {
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill(text)
  await page.getByRole('button', { name: 'Send' }).click()
}

// The bottom of the box sits above the sticky composer, inside the window.
async function inView(page: Page, text: string) {
  const box = (await bubbles(page).filter({ hasText: text }).boundingBox())!
  const composer = (await page.locator('.composer').boundingBox())!
  return box.y + box.height <= composer.y + 1 && box.y >= 0
}

test('Bubbles follow the prototype: player on the right, coach on the left, the advice category chip, readable type at 15px or larger with comfortable spacing, and the newest message kept in view', async ({
  page,
}) => {
  let release = () => {}
  const held = new Promise<void>((r) => (release = r))
  const slow: Reply = async (route, text) => {
    await held
    await ok(route, text)
  }
  await coachServer(page, [ok, slow], 12)
  await page.goto('/#/coach')
  await expect(bubbles(page)).toHaveCount(12)
  await send(page, 'viper keeps harassing me')
  const newest = bubbles(page).last()
  await expect(newest).toContainText('Lane · high priority')
  await expect.poll(() => inView(page, 'Coach on “viper')).toBe(true)

  const chatBox = (await chat(page).boundingBox())!
  const mine = (await bubbles(page).filter({ hasText: /^viper keeps harassing me$/ }).boundingBox())!
  const coach = (await newest.boundingBox())!
  // Player hugs the right edge, coach the left.
  expect(chatBox.x + chatBox.width - (mine.x + mine.width)).toBeLessThan(2)
  expect(coach.x - chatBox.x).toBeLessThan(2)
  expect(mine.y + mine.height).toBeLessThanOrEqual(coach.y - 8)
  const size = await newest.locator('.bubble-text').evaluate((el) => parseFloat(getComputedStyle(el).fontSize))
  expect(size).toBeGreaterThanOrEqual(15)

  // Scrolled up to read while the coach thinks, its answer leaves the view where it is.
  await page.getByRole('button', { name: 'Items now?' }).click()
  await expect(chat(page).getByRole('status')).toBeVisible()
  await page.evaluate(() => window.scrollTo(0, 0))
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0)
  release()
  await expect(bubbles(page).last()).toContainText('Coach on “Items now?”')
  expect(await page.evaluate(() => window.scrollY)).toBeLessThan(50)
  await page.getByRole('button', { name: 'New message ↓' }).click()
  await expect.poll(() => inView(page, 'Coach on “Items now?”')).toBe(true)
  await expect(page).toHaveScreenshot('coach-chat.png')
})

test('Quick actions above the composer (for example "Perdendo a lane", "Itens agora?", "Teamfight") send a message in one tap', async ({ page }) => {
  const sent = await coachServer(page, [ok])
  await page.addInitScript(() => window.localStorage.setItem('draftmind.locale', 'pt-BR'))
  await page.goto('/#/coach')
  const quick = page.getByRole('group', { name: 'Perguntas rápidas' })
  await expect(quick.getByRole('button')).toHaveText(['Perdendo a lane', 'Itens agora?', 'Teamfight', 'Próximo objetivo?'])
  // Above the composer.
  const chips = (await quick.boundingBox())!
  const field = (await page.getByRole('textbox', { name: 'Mensagem para o coach' }).boundingBox())!
  expect(chips.y + chips.height).toBeLessThanOrEqual(field.y)

  await quick.getByRole('button', { name: 'Perdendo a lane' }).click()
  await expect(bubbles(page, 'Conversa')).toHaveText(['Perdendo a lane', /Lane · prioridade alta\s*Coach on “Perdendo a lane”/])
  expect(sent).toEqual(['Perdendo a lane'])
  await expect(page).toHaveScreenshot('coach-quick-actions.png')
})

test('While the coach thinks or streams, a typing indicator shows and the input stays usable; errors show inline with a retry', async ({ page }) => {
  let release = () => {}
  const held = new Promise<void>((r) => (release = r))
  const slow: Reply = async (route, text) => {
    await held
    await ok(route, text)
  }
  const fail: Reply = (route) => route.fulfill({ status: 503, json: { error: 'down' } })
  const sent = await coachServer(page, [slow, fail, ok])
  await page.goto('/#/coach')

  await send(page, 'should I push top?')
  const typing = chat(page).getByRole('status')
  await expect(typing).toContainText('Thinking…')
  await expect(typing.locator('.typing-dots')).toBeVisible()
  const input = page.getByRole('textbox', { name: 'Message to the coach' })
  await input.fill('and after that?')
  await expect(input).toHaveValue('and after that?')
  await expect(page).toHaveScreenshot('coach-typing.png')
  release()
  await expect(typing).toHaveCount(0)
  await expect(bubbles(page).last()).toContainText('Coach on “should I push top?”')

  await page.getByRole('button', { name: 'Send' }).click()
  const failed = chat(page).getByRole('alert')
  await expect(failed).toContainText('Could not reach the coach')
  await expect(page).toHaveScreenshot('coach-error.png')
  await failed.getByRole('button', { name: 'Try again' }).click()
  await expect(failed).toHaveCount(0)
  await expect(bubbles(page).last()).toContainText('Coach on “and after that?”')
  // The retry resends the same message, which shows once.
  expect(sent).toEqual(['should I push top?', 'and after that?', 'and after that?'])
  await expect(bubbles(page).filter({ hasText: /^and after that\?$/ })).toHaveCount(1)
})

test('Phone (390x844) and desktop (1280x800) visual baselines match the new layout', async ({ page }) => {
  await coachServer(page, [ok], 4)
  await page.goto('/#/coach')
  await expect(bubbles(page)).toHaveCount(4)
  await page.getByRole('button', { name: 'Teamfight' }).click()
  await expect(bubbles(page)).toHaveCount(6)
  await expect(page).toHaveScreenshot('coach-layout.png', { fullPage: true })
})
