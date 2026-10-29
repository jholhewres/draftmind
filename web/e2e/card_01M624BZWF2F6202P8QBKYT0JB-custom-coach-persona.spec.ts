import type { Page } from '@playwright/test'
import { expect, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

// The coach answers each message; every request URL is kept.
async function coachServer(page: Page) {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  const asked: URL[] = []
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: session }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    asked.push(new URL(route.request().url()))
    const { text } = route.request().postDataJSON() as { text: string }
    return route.fulfill(
      sse([
        ['message', { id: 1, author: 'player', kind: 'text', text }],
        ['answer', { kind: 'answer', category: 'items', priority: 'high', source: 'jev', textSource: 'writer', text: 'Compre Black King Bar agora: o Axe vai te pegar nas lutas.' }],
        ['done', {}],
      ]),
    )
  })
  return asked
}

const choose = (page: Page, setting: string, value: string) =>
  page.getByRole('group', { name: setting }).getByRole('radio', { name: value }).check()

test("The coach answers in the persona's language", async ({ page }) => {
  const asked = await coachServer(page)
  await page.goto('/#/settings')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Coach settings')
  await choose(page, 'Language', 'Português (Brasil)')
  await choose(page, 'Tone', 'Analytic')
  await choose(page, 'Length', 'Short')
  await choose(page, 'Focus', 'Items')
  await expect(page.getByRole('status').first()).toHaveText('Saved on this device.')
  await expect(page).toHaveScreenshot('persona-settings.png', { fullPage: true })

  // The app stays in English; the coach is asked to answer in pt-BR.
  await page.getByRole('link', { name: 'Coach', exact: true }).click()
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('what do I buy vs axe')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByRole('region', { name: 'Conversation' }).getByRole('listitem').nth(1)).toContainText('Compre Black King Bar agora')
  expect(asked.map((u) => Object.fromEntries(u.searchParams))).toEqual([
    { lang: 'en', tone: 'analytic', language: 'pt-BR', verbosity: 'short', focus: 'items' },
  ])
  await expect(page.getByRole('region', { name: 'Conversation' })).toHaveScreenshot('persona-answer.png')
})

test('Settings belong to the signed-in player and survive a new device', async ({ page }) => {
  // A new device: nothing in this browser, the settings saved on the account.
  const puts: unknown[] = []
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.route('**/api/me/persona', (route) => {
    if (route.request().method() === 'PUT') {
      puts.push(route.request().postDataJSON())
      return route.fulfill({ json: route.request().postDataJSON() })
    }
    return route.fulfill({ json: { tone: 'encouraging', language: 'pt-BR', verbosity: 'normal', focus: 'macro' } })
  })
  await page.goto('/#/settings')
  await expect(page.getByRole('group', { name: 'Tone' }).getByRole('radio', { name: 'Encouraging' })).toBeChecked()
  await expect(page.getByRole('group', { name: 'Language' }).getByRole('radio', { name: 'Português (Brasil)' })).toBeChecked()
  await expect(page.getByRole('group', { name: 'Focus' }).getByRole('radio', { name: 'Macro' })).toBeChecked()
  await expect(page.getByText('Sign in to keep these settings on every device.')).toHaveCount(0)

  await choose(page, 'Focus', 'Teamfights')
  await expect(page.getByRole('status').first()).toHaveText('Saved to your account.')
  expect(puts).toEqual([{ tone: 'encouraging', language: 'pt-BR', verbosity: 'normal', focus: 'teamfights' }])
  expect(await page.evaluate(() => window.localStorage.getItem('draftmind.persona'))).toBeNull()
  await expect(page).toHaveScreenshot('persona-account.png', { fullPage: true })
})
