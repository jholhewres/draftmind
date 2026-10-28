import path from 'node:path'
import { test as base, expect, type Page } from '@playwright/test'

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)

// The CDN host the hero image paths point to (see heroImageBase in src/api.ts).
const imageCDN = 'https://cdn.cloudflare.steamstatic.com/**'

const backgroundImage = 'https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/backgrounds/**'
const googleFonts = 'https://fonts.googleapis.com/**'

type Fixtures = {
  // Requests that tried to leave the dev server unmocked; each was aborted.
  escaped: string[]
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
  // Registered after the guard, so these routes win.
  mockApi: [
    async ({ page, escaped }, provide) => {
      void escaped
      await page.route('**/api/health', (route) => route.fulfill({ path: fixture('health.json') }))
      await page.route('**/api/heroes', (route) => route.fulfill({ path: fixture('heroes.json') }))
      await page.route(imageCDN, (route) => route.fulfill({ path: fixture('hero-portrait.png') }))
      // Plain background color and fallback fonts keep the baselines stable.
      await page.route(backgroundImage, (route) => route.fulfill({ status: 404 }))
      await page.route(googleFonts, (route) => route.fulfill({ contentType: 'text/css', body: '' }))
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

export { expect }
