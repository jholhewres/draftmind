import type { Page } from '@playwright/test'
import { expect, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

// The live match the Coach tab restores, Axe's page, and a visitor's history.
async function mockScreens(page: Page) {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: session }))
  await page.route('**/api/me/matches', (route) => route.fulfill({ status: 401, json: { error: 'sign in first' } }))
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      if (url.pathname !== '/api/heroes/axe') return route.fulfill({ status: 404, json: { error: 'no such hero' } })
      return route.fulfill({ path: `${import.meta.dirname}/fixtures/hero-axe.en.json` })
    },
  )
}

// Each screen that is not a main tab, its back control's name and its parent.
const screens = [
  { hash: '#/settings', back: 'Back to the coach', parent: '#/coach' },
  { hash: '#/signin', back: 'Back', parent: '#/' },
  { hash: '#/heroes/axe', back: 'Back to heroes', parent: '#/heroes' },
  { hash: '#/coach/end', back: 'Back to the coach', parent: '#/coach' },
  { hash: '#/coach/history', back: 'Back to the coach', parent: '#/coach' },
  { hash: '#/draft/manual', back: 'Back to draft', parent: '#/draft' },
]

const hashOf = (page: Page) => page.evaluate(() => window.location.hash)

test('Every screen that is not a main tab has a visible back control that returns to the screen it came from, or to its parent tab on a deep link', async ({
  page,
}) => {
  await mockScreens(page)
  for (const s of screens) {
    // Opened from another tab: back returns there, not to the parent.
    await page.goto('/#/patches')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await page.evaluate((h) => (window.location.hash = h), s.hash)
    await page.getByRole('link', { name: s.back, exact: true }).click()
    await expect.poll(() => hashOf(page), { message: s.hash }).toBe('#/patches')

    // A deep link, opened from another site: back opens the parent and stays in the app.
    await page.goto('about:blank')
    await page.goto('/' + s.hash)
    await page.getByRole('link', { name: s.back, exact: true }).click()
    await expect.poll(() => hashOf(page), { message: s.hash }).toBe(s.parent)
  }
  await page.goto('/#/settings')
  await expect(page.getByRole('link', { name: 'Back to the coach' })).toBeVisible()
  await expect(page).toHaveScreenshot('settings-back.png')
})

test("Browser back and forward move through the app's screens in order, and a deep link opened directly has a back path that stays in the app", async ({
  page,
}) => {
  await mockScreens(page)
  await page.goto('/#/coach')
  await page.getByRole('link', { name: 'Coach settings' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Coach settings')
  await page.getByRole('link', { name: 'Heroes', exact: true }).click()
  await page.evaluate(() => (window.location.hash = '#/heroes/axe'))
  await expect(page.getByRole('heading', { level: 1, name: 'Axe' })).toBeVisible()

  await page.goBack()
  await expect.poll(() => hashOf(page)).toBe('#/heroes')
  await page.goBack()
  await expect.poll(() => hashOf(page)).toBe('#/settings')
  await page.goForward()
  await expect.poll(() => hashOf(page)).toBe('#/heroes')
  await page.goForward()
  await expect.poll(() => hashOf(page)).toBe('#/heroes/axe')

  // A deep link in a fresh tab: back opens the hero list, then its tab bar.
  const fresh = await page.context().newPage()
  await fresh.goto('/#/heroes/axe')
  await fresh.getByRole('link', { name: 'Back to heroes' }).click()
  await expect.poll(() => hashOf(fresh)).toBe('#/heroes')
  // The parent took the deep link's place, so it is still the tab's first screen.
  expect(await fresh.evaluate(() => (history.state as { dmDepth?: number }).dmDepth)).toBe(0)
  await expect(fresh.getByRole('navigation')).toBeVisible()
  await expect(fresh).toHaveScreenshot('deep-link-parent.png')
})

test('Modals and sheets (hero picker, confirmations) close with Esc, a close button and browser back, and return focus to the control that opened them', async ({
  page,
}) => {
  await page.goto('/#/draft/manual')
  const enemies = page.getByRole('group', { name: 'Enemies' })
  const opener = enemies.getByRole('button', { name: 'Add enemy' }).first()
  const dialog = page.getByRole('dialog', { name: 'Choose an enemy' })

  await opener.click()
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()

  await opener.click()
  await dialog.getByRole('button', { name: 'Close' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()

  await opener.click()
  await expect(dialog).toBeVisible()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Draft')
  await expect(page).toHaveScreenshot('picker-open.png')
  // Browser back closes the sheet and stays on the board.
  await expect.poll(() => page.evaluate(() => (history.state as { dmModal?: true } | null)?.dmModal)).toBe(true)
  await page.goBack()
  await expect(dialog).toHaveCount(0)
  await expect.poll(() => hashOf(page)).toBe('#/draft/manual')
  await expect(opener).toBeFocused()

  // Closing in the app leaves no extra entry: back leaves the board.
  await opener.click()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect.poll(() => page.evaluate(() => (history.state as { dmModal?: true } | null)?.dmModal ?? null)).toBeNull()
  // Forward onto the closed sheet's entry steps back off it.
  await page.goForward()
  await expect.poll(() => page.evaluate(() => (history.state as { dmModal?: true } | null)?.dmModal ?? null)).toBeNull()
  await expect(dialog).toHaveCount(0)
})
