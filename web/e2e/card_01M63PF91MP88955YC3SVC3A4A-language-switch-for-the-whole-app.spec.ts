import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)
const langOf = (url: URL) => (url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en')

// Axe's page and history in the language asked for; records each request.
async function mockAxe(page: Page) {
  const asked: string[] = []
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      asked.push(url.pathname + url.search)
      if (url.pathname === '/api/heroes/axe/history') return route.fulfill({ path: fixture(`history-axe.${langOf(url)}.json`) })
      if (url.pathname === '/api/heroes/axe/counters') return route.fulfill({ path: fixture('counters-axe.json') })
      if (url.pathname === '/api/heroes/axe') return route.fulfill({ path: fixture(`hero-axe.${langOf(url)}.json`) })
      return route.fulfill({ status: 404, json: { error: 'no such hero' } })
    },
  )
  return asked
}

test('A PT | EN switch in the footer and the account screen changes every UI string and the language of game text (hero details, patch notes) at once, without a reload', async ({
  page,
  escaped,
}) => {
  const asked = await mockAxe(page)
  await page.goto('/#/heroes/axe')
  await expect(page.getByText("Berserker's Call").first()).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  // A reload would drop this.
  await page.evaluate(() => ((window as unknown as { kept: boolean }).kept = true))

  const footer = page.getByRole('contentinfo')
  const lang = footer.getByRole('group', { name: 'Language' })
  await expect(lang.getByRole('button', { name: 'EN' })).toHaveAttribute('aria-pressed', 'true')
  await lang.getByRole('button', { name: 'PT' }).click()

  // UI strings, the hero's abilities and its patch notes, all in pt-BR.
  await expect(page.locator('html')).toHaveAttribute('lang', 'pt-BR')
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Heróis' }).first()).toBeVisible()
  await expect(page.getByText('Grito do Furioso').first()).toBeVisible()
  const history = page.getByRole('region', { name: 'Histórico' })
  await expect(history.getByText('Redução da Agilidade base (20 → 18)')).toBeVisible()
  await expect(footer.getByRole('group', { name: 'Idioma' }).getByRole('button', { name: 'PT' })).toHaveAttribute('aria-pressed', 'true')
  expect(asked).toContain('/api/heroes/axe?lang=pt-BR')
  expect(asked).toContain('/api/heroes/axe/history?lang=pt-BR')
  expect(await page.evaluate(() => (window as unknown as { kept?: boolean }).kept)).toBe(true)

  // The account screen has the same switch.
  await page.goto('/#/signin')
  const signin = page.getByRole('main')
  await expect(signin.getByRole('heading', { name: 'Entrar no Draftmind' })).toBeVisible()
  await page.getByRole('contentinfo').getByRole('group', { name: 'Idioma' }).getByRole('button', { name: 'EN' }).click()
  await expect(signin.getByRole('heading', { name: 'Sign in to Draftmind' })).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')

  await page.goto('/#/heroes/axe')
  await page.getByRole('contentinfo').getByRole('group', { name: 'Language' }).getByRole('button', { name: 'PT' }).click()
  await imagesLoaded(page.getByRole('main'))
  expect(escaped).toEqual([])
  await page.getByRole('contentinfo').scrollIntoViewIfNeeded()
  await expect(page.getByRole('contentinfo')).toHaveScreenshot('footer-switch-pt.png')
})

test("The choice persists in localStorage (and in the signed-in user's settings when one exists) and wins over the browser language", async ({
  page,
  escaped,
}) => {
  // The browser is en-US (playwright.config.ts).
  await page.goto('/#/patches')
  await page.getByRole('contentinfo').getByRole('group', { name: 'Language' }).getByRole('button', { name: 'PT' }).click()
  expect(await page.evaluate(() => localStorage.getItem('draftmind.locale'))).toBe('pt-BR')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('lang', 'pt-BR')
  await expect(page.getByRole('contentinfo').getByRole('group', { name: 'Idioma' })).toBeVisible()

  // Signed in on a new device, the player's saved language wins over the
  // browser's; switching on the account screen saves the new one.
  await page.evaluate(() => localStorage.clear())
  const saved: unknown[] = []
  let language = 'pt-BR'
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.route('**/api/me/language', (route) => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON() as { language: string }
      saved.push(body)
      language = body.language
      return route.fulfill({ json: body })
    }
    return route.fulfill({ json: { language } })
  })
  await page.goto('/#/signin')
  await page.reload()
  const main = page.getByRole('main')
  await expect(main.getByRole('heading', { name: 'Sua conta' })).toBeVisible()
  const lang = page.getByRole('contentinfo').getByRole('group', { name: 'Idioma' })
  await lang.getByRole('button', { name: 'EN' }).click()
  await expect(main.getByRole('heading', { name: 'Your account' })).toBeVisible()
  await expect.poll(() => saved).toEqual([{ language: 'en' }])
  expect(await page.evaluate(() => localStorage.getItem('draftmind.locale'))).toBe('en')

  expect(escaped).toEqual([])
  await expect(page.getByRole('contentinfo')).toHaveScreenshot('signin-footer-switch-en.png')
})
