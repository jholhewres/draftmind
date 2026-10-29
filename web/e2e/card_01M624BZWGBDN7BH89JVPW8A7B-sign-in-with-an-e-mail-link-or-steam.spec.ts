import type { Page } from '@playwright/test'
import { expect, test } from './fixtures.ts'

// The sign-in API as the server answers it: a link works once, /api/me
// knows the account once a link was spent and forgets it on sign-out.
// Returns the link requests.
async function mockSignIn(page: Page) {
  const asked: unknown[] = []
  let signedIn = false
  let opened = 0
  await page.route('**/api/me', (route) =>
    signedIn ? route.fulfill({ json: { email: 'player@example.com' } }) : route.fulfill({ status: 401, json: { error: 'sign in first' } }),
  )
  await page.route('**/api/auth/methods', (route) => route.fulfill({ json: { password: true, steam: true, emailLink: true } }))
  await page.route('**/api/auth/email', (route) => {
    asked.push(route.request().postDataJSON())
    return route.fulfill({ status: 202, json: { status: 'sent', minutes: 15 } })
  })
  await page.route('**/api/auth/email/verify', (route) => {
    if (opened++ > 0) return route.fulfill({ status: 401, json: { error: 'the link expired or was already used' } })
    signedIn = true
    return route.fulfill({ json: { email: 'player@example.com' } })
  })
  await page.route('**/api/auth/signout', (route) => {
    signedIn = false
    return route.fulfill({ status: 204 })
  })
  return asked
}

test('A magic link works once and expires after 15 minutes', async ({ page, escaped }) => {
  const asked = await mockSignIn(page)
  await page.goto('/#/draft')
  await page.getByRole('link', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/#\/signin$/)
  await expect(page.getByRole('heading', { name: 'Sign in to Draftmind' })).toBeVisible()
  await expect(page).toHaveScreenshot('signin.png', { fullPage: true })

  await page.getByLabel('E-mail').fill('player@example.com')
  await page.getByRole('button', { name: 'Send sign-in link' }).click()
  await expect(page.getByText('Check your inbox: we sent a link to player@example.com. It works once, for 15 minutes.')).toBeVisible()
  expect(asked).toEqual([{ email: 'player@example.com', lang: 'en' }])

  await page.goto('/#/signin/link/tok_1')
  await expect(page.getByText('Signed in as player@example.com')).toBeVisible()
  await expect(page).toHaveURL(/#\/signin$/)
  await expect(page).toHaveScreenshot('signin-signed-in.png', { fullPage: true })

  await page.goto('/#/draft')
  await expect(page.getByRole('link', { name: 'Account' })).toBeVisible()

  await page.goto('/#/signin')
  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.goto('/#/signin/link/tok_1')
  await expect(page.getByText('This link has expired or was already used. Ask for a new one below.')).toBeVisible()
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('signin-link-used.png', { fullPage: true })
})

test('A Steam OpenID assertion is verified against Steam, tested with an httptest server', async ({ page, escaped }) => {
  await mockSignIn(page)
  await page.goto('/#/signin/failed')
  await expect(page.getByText('Steam sign-in did not finish. Try again.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Continue with Steam' })).toHaveAttribute('href', '/api/auth/steam')
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('signin-steam-failed.png', { fullPage: true })
})

test('Endpoints that need a session answer 401 without one', async ({ page, escaped }) => {
  await page.goto('/#/draft')
  // /api/me answers 401: the header offers sign-in and the draft still works.
  await expect(page.getByRole('link', { name: 'Sign in' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Photograph the pick' })).toBeVisible()
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('visitor-header.png')
})
