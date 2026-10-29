import path from 'node:path'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, test } from './fixtures.ts'
import heroes from './fixtures/heroes.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

// Axe's page with its popular build, and the build against a lineup; the
// builds come from item rows the current patch has.
async function mockBuild(page: Page, builds: unknown[]) {
  await page.route(
    (url) => url.pathname.startsWith('/api/heroes/'),
    (route) => {
      const url = new URL(route.request().url())
      const lang = url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en'
      if (url.pathname === '/api/heroes/axe/build') return route.fulfill({ path: fixture('build-axe.json') })
      if (url.pathname === '/api/heroes/axe') return route.fulfill({ path: fixture(`hero-axe.${lang}.json`) })
      return route.fulfill({ status: 404, json: { error: 'no such hero' } })
    },
  )
  await page.route('**/api/draft/build', (route) => {
    builds.push(route.request().postDataJSON())
    return route.fulfill({ path: fixture('draft-build.json') })
  })
}

// An Axe match against Crystal Maiden, Bane and Drow Ranger, its item plan
// decided when it started; the Coach tab restores it by the kept id.
async function axeMatch(page: Page) {
  const id = '0b9e8d7c-6a5f-4e3d-9c2b-1a0f9e8d7c6b'
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), id)
  await page.route(`**/api/match/${id}`, (route) => route.fulfill({ path: fixture('match-axe.json') }))
}

test('Only items that exist in the current patch are suggested', async ({ page, escaped }) => {
  await mockBuild(page, [])
  await page.goto('/#/heroes/axe')

  const build = page.getByRole('region', { name: 'Popular build · patch 7.41' })
  await expect(build.getByRole('list', { name: 'Starting' }).getByRole('listitem')).toHaveText(['Tango90 gold', 'Ring of Protection175 gold', 'Iron Branch50 gold'])
  await expect(build.getByRole('list', { name: 'Late game' }).getByRole('listitem')).toHaveText(['Black King Bar4,050 gold', 'Overwhelming Blink6,550 gold'])
  await expect(build.getByRole('listitem').filter({ hasText: 'Tango' })).toHaveAttribute('title', '51,200 games')
  await expect(build.getByText('From OpenDota item popularity.')).toBeVisible()

  await imagesLoaded(page.locator('.hero-banner'))
  expect(escaped).toEqual([])
  await expect(build).toHaveScreenshot('hero-popular-build.png')
})

test('Each situational item cites the enemy hero that triggers it', async ({ page, escaped }) => {
  await axeMatch(page)
  await page.goto('/#/coach')

  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Axe · Offlane')
  const card = page.getByRole('region', { name: 'Build · against this lineup' })
  const name = (id: number) => heroes.heroes.find((h) => h.id === id)!.name
  await expect(card.getByRole('list', { name: 'Situational items' }).getByRole('listitem')).toHaveText([
    `Black King Bar4,050 goldnow · vs ${name(5)}`,
    `Lotus Orb3,850 goldsoon · vs ${name(3)}`,
    `Force Staff2,200 goldlater · vs ${name(6)}`,
  ])
  await expect(card.getByRole('list', { name: 'Mid game' }).getByRole('listitem')).toHaveText(['Blink Dagger2,250 gold', 'Blade Mail2,100 gold'])

  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('coach-build.png')
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('The build card reads in the player language', async ({ page }) => {
    await axeMatch(page)
    await page.goto('/#/coach')
    const card = page.getByRole('region', { name: 'Build · contra esta formação' })
    await expect(card.getByRole('listitem').first()).toHaveText('Black King Bar4.050 de ouroagora · vs Crystal Maiden')
    await expect(card.getByText('Da popularidade de itens do OpenDota.')).toBeVisible()
  })
})
