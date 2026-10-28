import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, portraitsLoaded, test } from './fixtures.ts'

// Axe's page as GET /api/heroes/axe returns it, captured from the real API
// after a sync; any other hero is not on the patch. Records each request (the
// dev server's StrictMode sends the first one twice).
async function mockHero(page: Page) {
  const asked: string[] = []
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      asked.push(url.pathname + url.search)
      if (url.pathname !== '/api/heroes/axe') return route.fulfill({ status: 404, json: { error: 'no such hero' } })
      const lang = url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en'
      return route.fulfill({ path: path.join(import.meta.dirname, 'fixtures', `hero-axe.${lang}.json`) })
    },
  )
  return asked
}

async function bannerLoaded(page: Page) {
  const img = page.locator('.hero-banner img')
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth)).toBe(128)
}

test("GET /api/heroes/{slug} returns the current patch's stats and abilities in the requested language", async ({
  page,
  escaped,
}, testInfo) => {
  const asked = await mockHero(page)
  await page.goto('/#/heroes')
  await portraitsLoaded(page)
  await page.getByRole('list', { name: 'Heroes' }).getByRole('link', { name: 'Axe' }).click()

  await expect(page).toHaveURL(/#\/heroes\/axe$/)
  await expect(page.getByRole('heading', { level: 1, name: 'Axe' })).toBeVisible()
  await expect(page.getByText('Strength · Melee · Initiator, Durable, Disabler, Carry')).toBeVisible()
  const stats = page.getByRole('region', { name: 'Stats' })
  await expect(stats.getByText('Health')).toBeVisible()
  const abilities = page.getByRole('region', { name: 'Abilities' })
  await expect(abilities.getByRole('heading', { level: 3 })).toHaveText([
    "Berserker's Call",
    'Battle Hunger',
    'Counter Helix',
    'Culling Blade',
    'One Man Army',
  ])
  await expect(abilities.getByText('Cooldown: 18 / 16 / 14 / 12')).toBeVisible()
  expect(new Set(asked)).toEqual(new Set(['/api/heroes/axe?lang=en']))

  // Desktop: stats and abilities side by side; phone: stacked.
  const statsBox = (await stats.boundingBox())!
  const abilitiesBox = (await abilities.boundingBox())!
  if (testInfo.project.name === 'desktop') expect(abilitiesBox.x).toBeGreaterThan(statsBox.x + statsBox.width)
  else expect(abilitiesBox.y).toBeGreaterThan(statsBox.y + statsBox.height)

  await bannerLoaded(page)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('hero-page-en.png', { fullPage: true })
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('The hero page renders the abilities in Portuguese when the UI is in pt-BR', async ({ page, escaped }) => {
    const asked = await mockHero(page)
    await page.goto('/#/heroes/axe')

    const abilities = page.getByRole('region', { name: 'Habilidades' })
    await expect(abilities.getByRole('heading', { level: 3 })).toHaveText([
      'Grito do Furioso',
      'Fome de Batalha',
      'Contraespiral',
      'Lâmina de Abate',
      'Exército de Um Homem Só',
    ])
    await expect(abilities.getByText('Recarga: 18 / 16 / 14 / 12')).toBeVisible()
    await expect(page.getByText('Força · Corpo a corpo · Iniciador, Resistente, Desabilitador, Carregador')).toBeVisible()
    await expect(page.getByRole('region', { name: 'Atributos' }).getByText('Vida')).toBeVisible()
    expect(new Set(asked)).toEqual(new Set(['/api/heroes/axe?lang=pt-BR']))

    await bannerLoaded(page)
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('hero-page-pt-br.png', { fullPage: true })
  })
})

test('A hero the current patch does not have shows a not-found message', async ({ page, escaped }) => {
  await mockHero(page)
  await page.goto('/#/heroes/nobody')

  await expect(page.getByText('No hero “nobody” on the current patch.')).toBeVisible()
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('hero-page-not-found.png')
})
