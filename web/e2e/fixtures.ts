import path from 'node:path'
import { test as base, expect, type Locator, type Page } from '@playwright/test'
import ranking from './fixtures/suggest.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

// The CDN host the hero image paths point to (see heroImageBase in src/api.ts).
const imageCDN = 'https://cdn.cloudflare.steamstatic.com/**'

const backgroundImage = 'https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/backgrounds/**'
// The landing page's hero art (see heroArt in src/landing/Landing.tsx).
const heroArt = 'https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/home/radiant_dire5.jpg'

export type DraftInput = { allies: number[]; enemies: number[]; bans: number[]; role: number }

// The fixture ranking without the heroes the draft took, each keeping its
// deltas against the draft's enemies only.
export function rankFor(draft: DraftInput) {
  const taken = new Set([...draft.allies, ...draft.enemies, ...draft.bans])
  return ranking.suggestions
    .filter((s) => !taken.has(s.heroId))
    .map((s) => ({ ...s, enemies: s.enemies.filter((e) => draft.enemies.includes(e.heroId)) }))
}

// A whole Server-Sent Events stream as one response, for route.fulfill.
export const sse = (events: [string, unknown][]) => ({
  contentType: 'text/event-stream',
  body: events.map(([name, data]) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`).join(''),
})

// GET /api/sync/status with 7.41 current, last synced hoursAgo before now.
export const syncStatus = (hoursAgo: number) => {
  const at = new Date(Date.now() - hoursAgo * 3600_000).toISOString()
  return {
    patch: '7.41',
    updatedAt: at,
    sources: [
      { source: 'dotaconstants:heroes', lastSuccess: at },
      { source: 'opendota:stats', lastSuccess: at },
    ],
    lastFailure: null,
  }
}

type Fixtures = {
  // Requests that tried to leave the dev server unmocked; each was aborted.
  escaped: string[]
  // Each draft POST /api/draft/stream was asked to rank, in order.
  drafts: DraftInput[]
  mockApi: void
}

export const test = base.extend<Fixtures>({
  escaped: async ({ page, baseURL }, provide) => {
    const escaped: string[] = []
    await page.route('**/*', (route) => {
      const url = route.request().url()
      if (url.startsWith(baseURL!)) return route.fallback()
      escaped.push(url)
      return route.abort()
    })
    await provide(escaped)
  },
  // Streams the fixture ranking for each draft, kept as Jev's order.
  drafts: async ({ page, escaped }, provide) => {
    void escaped
    const drafts: DraftInput[] = []
    await page.route('**/api/draft/stream**', (route) => {
      const draft = route.request().postDataJSON() as DraftInput
      drafts.push(draft)
      const suggestions = rankFor(draft)
      return route.fulfill(
        sse([
          ['ranking', { patch: ranking.patch, suggestions }],
          ['decision', { source: 'jev', confidence: 0.4, suggestions }],
          ['done', {}],
        ]),
      )
    })
    await provide(drafts)
  },
  // Registered after the guard, so these routes win.
  mockApi: [
    async ({ page, drafts }, provide) => {
      void drafts
      await page.route('**/api/health', (route) => route.fulfill({ path: fixture('health.json') }))
      // Synced three hours before the request, so the footer reads the same every run.
      await page.route('**/api/sync/status', (route) => route.fulfill({ json: syncStatus(3) }))
      // A visitor unless a spec signs in.
      await page.route('**/api/me', (route) => route.fulfill({ status: 401, json: { error: 'sign in first' } }))
      // Production today: password and Steam, no mailer for e-mail links.
      await page.route('**/api/auth/methods', (route) => route.fulfill({ json: { password: true, steam: true, emailLink: false } }))
      await page.route('**/api/heroes', (route) => route.fulfill({ path: fixture('heroes.json') }))
      await page.route('**/api/patches', (route) => route.fulfill({ path: fixture('patches.json') }))
      await page.route('**/api/patches/*', (route) => {
        const url = new URL(route.request().url())
        const lang = url.searchParams.get('lang') === 'pt-BR' ? 'pt-BR' : 'en'
        return route.fulfill({ path: fixture(`patch-${decodeURIComponent(url.pathname.split('/').at(-1)!)}.${lang}.json`) })
      })
      await page.route(imageCDN, (route) => route.fulfill({ path: fixture('hero-portrait.png') }))
      // A plain background color keeps the baselines stable; the fonts are
      // self-hosted, so a font request would escape and fail the spec.
      await page.route(backgroundImage, (route) => route.fulfill({ status: 404 }))
      await page.route(heroArt, (route) => route.fulfill({ path: fixture('hero-art.jpg') }))
      await provide()
    },
    { auto: true },
  ],
})

// Waits until every portrait in the hero grid has loaded from the local
// fixture (128px wide), then scrolls back to the top.
export async function portraitsLoaded(page: Page) {
  for (const img of await page.getByRole('list', { name: 'Heroes' }).locator('img').all()) {
    await img.scrollIntoViewIfNeeded()
    await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth)).toBe(128)
  }
  await page.evaluate(() => window.scrollTo(0, 0))
}

// Waits until every image under root has loaded from the local fixture.
export async function imagesLoaded(root: Locator) {
  for (const img of await root.locator('img').all()) {
    await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth)).toBe(128)
  }
}

export { expect }
