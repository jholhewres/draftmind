import type { Page } from '@playwright/test'
import { previewURL } from '../playwright.config.ts'
import { expect, imagesLoaded, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

// Every CSP violation the page reports.
async function violations(page: Page) {
  const seen: string[] = []
  await page.exposeFunction('cspViolation', (v: string) => seen.push(v))
  await page.addInitScript(() =>
    document.addEventListener('securitypolicyviolation', (e) =>
      (window as unknown as { cspViolation: (v: string) => void }).cspViolation(`${e.violatedDirective} ${e.blockedURI}`),
    ),
  )
  return seen
}

test("Every response from the binary carries a Content-Security-Policy that allows only the app's own scripts and the image CDN, plus X-Content-Type-Options, Referrer-Policy and a frame-ancestors deny; the app still loads hero and item images, fonts and the screen share", async ({
  page,
}) => {
  const seen = await violations(page)
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  const res = await page.goto(`${previewURL}/#/heroes`)
  const csp = res!.headers()['content-security-policy']
  expect(csp).toContain("script-src 'self'")
  expect(csp).toContain("img-src 'self' https://cdn.cloudflare.steamstatic.com")
  expect(csp).toContain("frame-ancestors 'none'")

  // Hero portraits from the image CDN and the self-hosted fonts load.
  const portraits = page.getByRole('list', { name: 'Heroes' }).locator('img')
  await expect(portraits.first()).toBeVisible()
  await imagesLoaded(portraits.first().locator('xpath=..'))
  await expect.poll(() => page.evaluate(() => document.fonts.check('600 16px Barlow'))).toBe(true)
  await expect.poll(() => page.evaluate(() => [...document.fonts].some((f) => f.status === 'loaded'))).toBe(true)

  // Item cards in the coach, from the same CDN.
  await page.goto(`${previewURL}/#/coach`)
  const build = page.getByRole('region', { name: /^Build/ })
  await expect(build.locator('img').first()).toBeVisible()
  await imagesLoaded(build)
  expect(seen).toEqual([])
  await expect(page).toHaveScreenshot('csp-coach.png')
})

test('Each match session and each anonymous client has a model spend cap (an env setting with a default) from `model_call` costs. Past the cap, model calls stop with a clear message, and the code ranking and templates keep working', async ({
  page,
}) => {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  await page.route(`**/api/match/${session.id}/messages**`, (route) =>
    route.fulfill({
      contentType: 'text/event-stream',
      body: [
        `event: message\ndata: ${JSON.stringify({ id: 9, author: 'player', kind: 'text', text: 'what now?' })}\n\n`,
        `event: answer\ndata: ${JSON.stringify({ kind: 'answer', textSource: 'template', reason: 'budget' })}\n\n`,
        'event: done\ndata: {}\n\n',
      ].join(''),
    }),
  )
  await page.goto('/#/coach')
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('what now?')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.locator('.bubble.coach').last()).toContainText('The AI spend cap for now is used up.')
  await expect(page).toHaveScreenshot('budget-template.png')

  // A photo read past the cap says so, and the board can still be built by hand.
  await page.route('**/api/draft/stream**', (route) => route.fulfill({ status: 429, headers: { 'X-Spend-Cap': 'used' }, json: { error: 'cap' } }))
  await page.goto('/#/draft')
  await page.locator('input[type=file]').first().setInputFiles('e2e/fixtures/pick-photo.jpg')
  await expect(page.getByRole('alert')).toContainText('The AI spend cap for now is used up.')
  await expect(page.getByRole('link', { name: /by hand/i })).toBeVisible()
})

test('The live session shows how much of the cap is used', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === 'phone', 'screen sharing is desktop only')
  test.setTimeout(60_000)
  await page.addInitScript(() => {
    window.localStorage.setItem('draftmind.liveTick', '10')
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas')
      c.width = 1280
      c.height = 720
      const ctx = c.getContext('2d')!
      // The world brightens step by step, so every tick sees a change.
      let n = 60
      window.setInterval(() => {
        n = n >= 220 ? 60 : n + 1
        ctx.fillStyle = `rgb(${n},${n},${n})`
        ctx.fillRect(0, 0, 1280, 620)
        ctx.fillStyle = '#141414'
        ctx.fillRect(0, 620, 1280, 100)
      }, 100)
      const stream = c.captureStream(5)
      const track = stream.getVideoTracks()[0]
      track.getSettings = () => ({ displaySurface: 'window' }) as MediaTrackSettings
      return stream
    }
  })
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  let ticks = 0
  await page.route(`**/api/match/${session.id}/live**`, (route) => {
    ticks++
    const out = ticks > 1
    return route.fulfill({ json: { speak: false, reason: out ? 'budget' : 'silent', cost: out ? undefined : 0.1, budget: { spent: out ? 0.25 : 0.1, cap: 0.25 } } })
  })
  await page.goto('/#/coach')
  const bar = page.getByRole('region', { name: 'Live coach' })
  await bar.getByRole('button', { name: 'Share game window' }).click()
  await expect(bar).toContainText('AI budget $0.10 of $0.25', { timeout: 15_000 })
  await expect(bar).toContainText('AI budget used up: the built-in answers keep working', { timeout: 25_000 })
  await expect(bar).toHaveScreenshot('budget-used-up.png')
})
