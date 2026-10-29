import { expect, imagesLoaded, rankFor, sse, test } from './fixtures.ts'
import read from './fixtures/read.json' with { type: 'json' }
import session from './fixtures/match.json' with { type: 'json' }

test("Retry on a failed coach message resends it without adding a second copy of the player's message to the chat", async ({ page }) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  const bodies: { text: string; resend?: number }[] = []
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const body = route.request().postDataJSON() as { text: string; resend?: number }
    bodies.push(body)
    const stored = ['message', { id: 41, author: 'player', kind: 'text', text: body.text }] as [string, unknown]
    // The first stream stores the message, then breaks before the answer.
    if (bodies.length === 1) return route.fulfill(sse([stored]))
    return route.fulfill(
      sse([stored, ['answer', { kind: 'answer', category: 'lane', priority: 'high', textSource: 'writer', text: 'Pull the side wave.' }], ['done', {}]]),
    )
  })
  await page.goto('/#/coach')
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('should I push top?')
  await page.getByRole('button', { name: 'Send' }).click()
  // A stream that ends before its done event is a failed turn.
  const chat = page.getByRole('region', { name: 'Conversation' })
  const failed = chat.getByRole('alert')
  await expect(failed).toContainText('Could not reach the coach')
  await failed.getByRole('button', { name: 'Try again' }).click()
  await expect(failed).toHaveCount(0)
  await expect(chat.locator('.bubble').last()).toContainText('Pull the side wave.')
  expect(bodies[1]).toEqual({ text: 'should I push top?', resend: 41 })
  await expect(chat.locator('.bubble.mine').filter({ hasText: /^should I push top\?$/ })).toHaveCount(1)
  await expect(page).toHaveScreenshot('retry-once.png')
})

test('The photo and live draft screens show ban suggestions in Captains Mode, as the typed draft does', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('draftmind.mode', 'captains_mode'))
  await page.route('**/api/draft/stream**', (route) => {
    const multipart = route.request().headers()['content-type']?.startsWith('multipart/form-data')
    const mode = new URL(route.request().url()).searchParams.get('mode') ?? (route.request().postDataJSON() as { mode?: string }).mode
    const seated = { allies: [5, 3], enemies: [2, 7], bans: [], role: 1 }
    const suggestions = rankFor(seated)
    const bans = mode === 'captains_mode' ? suggestions.slice(0, 2).map((s) => ({ ...s, counter: 4.2 })) : undefined
    return route.fulfill(
      sse([
        ...(multipart ? [['read', { ...read, draft: seated }] as [string, unknown]] : []),
        ['ranking', { patch: read.patch, suggestions, bans, mode, stats: 'ranked' }],
        ['decision', { source: 'jev', confidence: 0.4, suggestions }],
        ['done', {}],
      ]),
    )
  })
  await page.goto('/#/draft')
  await page.locator('input[type=file]').first().setInputFiles('e2e/fixtures/pick-photo.jpg')
  const bans = page.getByRole('list', { name: 'Ban suggestions' })
  await expect(bans.getByRole('listitem')).toHaveCount(2)
  await imagesLoaded(page.locator('main'))
  await expect(bans).toHaveScreenshot('photo-bans.png')
})
