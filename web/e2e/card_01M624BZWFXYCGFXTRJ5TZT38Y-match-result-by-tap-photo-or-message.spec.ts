import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)
const notes = ['Axe won the lane until you stayed behind your creeps.', 'Black King Bar turned the fights against Axe.']

// The match server for the end screen: messages and photos are read into a
// result, and every confirmed result is kept in saved.
async function endServer(page: Page) {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  const saved: unknown[] = []
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: session }))
  await page.route(`**/api/match/${session.id}/end/parse`, (route) => {
    const { text } = route.request().postDataJSON() as { text: string }
    if (text === 'perdemos') return route.fulfill({ json: { result: 'loss' } })
    return route.fulfill({ status: 422, json: { error: 'the message does not say how the match ended' } })
  })
  await page.route(`**/api/match/${session.id}/end/read`, (route) =>
    route.fulfill({ json: { result: 'win', durationS: 2292, kills: 9, deaths: 3, assists: 14 } }),
  )
  await page.route(`**/api/match/${session.id}/result**`, (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    saved.push(body)
    return route.fulfill({ json: { ...body, notes, endedAt: '2026-10-29T03:00:00Z' } })
  })
  return saved
}

async function openEnd(page: Page) {
  await page.goto('/#/coach')
  await page.getByRole('link', { name: 'End match' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('End of match')
}

test('A tap or a message saying the match was won or lost records the result', async ({ page }) => {
  const saved = await endServer(page)
  await openEnd(page)

  // A message is read into the result; the player still confirms it.
  await page.getByRole('textbox', { name: 'How did it end?' }).fill('gg')
  await page.getByRole('button', { name: 'Read', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('That message does not say who won. Tap Win or Loss.')
  await page.getByRole('textbox', { name: 'How did it end?' }).fill('perdemos')
  await page.getByRole('button', { name: 'Read', exact: true }).click()
  await expect(page.getByRole('radio', { name: 'Loss' })).toBeChecked()
  expect(saved).toEqual([])
  await page.getByRole('button', { name: 'Save result' }).click()
  await expect(page.getByRole('status').first()).toHaveText('Loss with Juggernaut: saved.')
  expect(saved).toEqual([{ result: 'loss', source: 'message' }])
  await expect(page.getByRole('region', { name: 'What the coach noted' }).getByRole('listitem')).toHaveText(notes)

  // The ended match leaves the Coach tab; a tap ends the next one.
  await page.getByRole('link', { name: 'Coach' }).click()
  await expect(page.getByText('Pick a hero on the Draft tab to start a match.')).toBeVisible()
  await page.reload()
  await openEnd(page)
  await page.getByRole('radio', { name: 'Win' }).check()
  await page.getByRole('button', { name: 'Save result' }).click()
  await expect(page.getByRole('status').first()).toHaveText('Win with Juggernaut: saved.')
  expect(saved.at(-1)).toEqual({ result: 'win', source: 'tap' })
  await expect(page).toHaveScreenshot('result-saved.png', { fullPage: true })
})

test('A photo of the end-screen fixture fills result, duration and KDA, and nothing is saved until the player confirms', async ({ page }) => {
  const saved = await endServer(page)
  await openEnd(page)

  await page.getByLabel('Photo of the end screen').setInputFiles(fixture('end-screen.jpg'))
  await expect(page.getByRole('heading', { name: /Read from the photo/ })).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Win' })).toBeChecked()
  await expect(page.getByRole('textbox', { name: 'Duration' })).toHaveValue('38:12')
  await expect(page.getByRole('spinbutton', { name: 'Kills' })).toHaveValue('9')
  await expect(page.getByRole('spinbutton', { name: 'Deaths' })).toHaveValue('3')
  await expect(page.getByRole('spinbutton', { name: 'Assists' })).toHaveValue('14')
  expect(saved).toEqual([])
  await expect(page).toHaveScreenshot('result-from-photo.png', { fullPage: true })

  // The player fixes the deaths, then confirms.
  await page.getByRole('spinbutton', { name: 'Deaths' }).fill('4')
  await page.getByRole('button', { name: 'Save result' }).click()
  await expect(page.getByRole('status').first()).toHaveText('Win with Juggernaut: saved.')
  expect(saved).toEqual([{ result: 'win', source: 'photo', durationS: 2292, kills: 9, deaths: 4, assists: 14 }])
})

test("The history lists the player's matches with hero and result, newest first", async ({ page }) => {
  // A visitor is asked to sign in.
  await page.route('**/api/me/matches', (route) => route.fulfill({ status: 401, json: { error: 'sign in first' } }))
  await page.goto('/#/coach/history')
  await expect(page.getByText('Your history is kept once you sign in.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Sign in' }).last()).toHaveAttribute('href', '#/signin')

  await page.unroute('**/api/me/matches')
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.route('**/api/me/matches', (route) =>
    route.fulfill({
      json: [
        { id: 'b', heroId: 8, role: 1, result: 'win', durationS: 2292, kills: 9, deaths: 4, assists: 14, endedAt: '2026-10-28T22:10:00Z' },
        { id: 'a', heroId: 2, role: 3, result: 'loss', endedAt: '2026-10-27T19:00:00Z' },
      ],
    }),
  )
  await page.reload()
  const list = page.getByRole('list', { name: 'Match history' })
  await expect(list.getByRole('listitem')).toHaveText([/^Juggernaut\s*Win\s*Oct 28, 2026 · 38:12 · 9\/4\/14$/, /^Axe\s*Loss\s*Oct 27, 2026$/])
  await imagesLoaded(list)
  await expect(page).toHaveScreenshot('history.png', { fullPage: true })
})
