import { expect, imagesLoaded, test } from './fixtures.ts'

const note =
  "Draftmind is an independent tool. It has no relation to Valve Corporation or the game and is not endorsed by them; Dota 2 is a trademark of Valve. Draftmind runs on a separate device or browser tab: it never reads the game's memory, injects into the client or draws an overlay over the game. Its only inputs are what you type, say or photograph. Hero names and images belong to their owners."

test('The three sections and the sign-in call to action render', async ({ page, escaped }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Your draft, read in seconds.')
  await expect(page.getByRole('heading', { level: 2 })).toHaveText(['How it works', 'Always on the current patch'])
  await expect(page.getByRole('heading', { level: 3 })).toHaveText(['Photograph the pick', 'Choose among the suggestions', 'Play with the coach'])
  await expect(page.locator('.landing-bullets li')).toHaveCount(3)
  const portraits = page.locator('.landing-portraits')
  await expect(portraits.locator('img')).toHaveCount(6)
  await imagesLoaded(portraits)
  // The hero art is the local stand-in for the CDN image, fully loaded.
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.querySelector('.landing-hero')!).backgroundImage)).toContain('radiant_dire5.jpg')

  await expect(page.getByRole('link', { name: 'Sign in' }).first()).toHaveAttribute('href', '#/signin')
  await expect(page.getByRole('link', { name: 'Create account' })).toHaveAttribute('href', '#/signin')
  // The fonts are served by the app itself, and the favicon is the gold D.
  const loaded = await page.evaluate(async () => {
    const faces = await Promise.all(['700 16px Cinzel', '400 16px Barlow', '600 16px "Barlow Condensed"'].map((f) => document.fonts.load(f)))
    return faces.map((f) => f.length)
  })
  expect(loaded).toEqual([1, 1, 1])
  expect(await (await page.request.get('/favicon.svg')).text()).toContain('#D9AE5B')
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('landing.png', { fullPage: true })

  // The call to action opens the app.
  await page.getByRole('link', { name: 'Start free' }).click()
  await expect(page).toHaveURL(/#\/draft$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Photograph the pick')
})

test('A signed-in player at the root goes to the app', async ({ page, escaped }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Photograph the pick')
  await expect(page.getByRole('link', { name: 'Account' })).toBeVisible()
  expect(escaped).toEqual([])
})

test('The trademark and fair-play note is in the footer', async ({ page, escaped }) => {
  await page.goto('/')
  const footer = page.getByRole('contentinfo')
  await expect(footer.getByText(note)).toBeVisible()
  await expect(footer.getByRole('navigation', { name: 'Footer' }).getByRole('link')).toHaveText(['Heroes', 'Patches', 'Sign in'])
  await footer.scrollIntoViewIfNeeded()
  expect(escaped).toEqual([])
  await expect(footer).toHaveScreenshot('landing-footer.png')
})

test('The page renders without horizontal scroll at 360 px wide', async ({ page, escaped }) => {
  await page.setViewportSize({ width: 360, height: 780 })
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await imagesLoaded(page.locator('.landing-portraits'))
  const { scroll, client } = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }))
  expect(scroll).toBeLessThanOrEqual(client)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('landing-360.png', { fullPage: true })
})
