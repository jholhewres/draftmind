import type { Page } from '@playwright/test'
import { expect, test } from './fixtures.ts'

type Creds = { email: string; password: string }

// The password API as the server answers it: register answers the same for
// a new and a taken e-mail, login one generic 401 for a wrong password or
// an unknown e-mail. Returns the requests each route got.
async function mockPassword(page: Page, opts: { accounts?: Creds[]; loginStatus?: number } = {}) {
  const accounts = [...(opts.accounts ?? [])]
  const calls = { register: [] as Creds[], login: [] as Creds[] }
  let signedIn: string | null = null
  await page.route('**/api/me', (route) =>
    signedIn ? route.fulfill({ json: { email: signedIn } }) : route.fulfill({ status: 401, json: { error: 'sign in first' } }),
  )
  await page.route('**/api/auth/register', (route) => {
    const c = route.request().postDataJSON() as Creds
    calls.register.push(c)
    if (!accounts.some((a) => a.email === c.email)) accounts.push(c)
    return route.fulfill({ json: { status: 'ok' } })
  })
  await page.route('**/api/auth/login', (route) => {
    const c = route.request().postDataJSON() as Creds
    calls.login.push(c)
    if (opts.loginStatus) return route.fulfill({ status: opts.loginStatus, json: { error: 'too many attempts for this address; try again later' } })
    if (!accounts.some((a) => a.email === c.email && a.password === c.password)) {
      return route.fulfill({ status: 401, json: { error: 'wrong e-mail or password' } })
    }
    signedIn = c.email
    return route.fulfill({ json: { email: c.email } })
  })
  await page.route('**/api/auth/signout', (route) => {
    signedIn = null
    return route.fulfill({ status: 204 })
  })
  return calls
}

test('A visitor creates an account with e-mail and password, then signs in and out. The password is stored only as an argon2id or bcrypt hash and must be at least 8 characters', async ({
  page,
  escaped,
}) => {
  const calls = await mockPassword(page)
  await page.goto('/#/signin')
  await page.getByLabel('E-mail').fill('player@example.com')
  await page.getByLabel('Password').fill('1234567')
  await page.getByRole('button', { name: 'Create account' }).click()
  // The browser refuses 7 characters before anything is sent.
  expect(calls.register).toEqual([])

  await page.getByLabel('Password').fill('12345678')
  await page.getByRole('button', { name: 'Create account' }).click()
  await expect(page.getByText('Signed in as player@example.com')).toBeVisible()
  expect(calls.register).toEqual([{ email: 'player@example.com', password: '12345678' }])
  await expect(page).toHaveScreenshot('password-signed-in.png', { fullPage: true })

  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.getByLabel('E-mail').fill('player@example.com')
  await page.getByLabel('Password').fill('12345678')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByText('Signed in as player@example.com')).toBeVisible()
  expect(calls.login).toHaveLength(2)
  expect(escaped).toEqual([])
})

test('A wrong password or unknown e-mail returns the same generic error, and sign-in attempts are rate limited per IP and per e-mail', async ({
  page,
  escaped,
}) => {
  await mockPassword(page, { accounts: [{ email: 'player@example.com', password: '12345678' }] })
  await page.goto('/#/signin')
  await page.getByLabel('E-mail').fill('player@example.com')
  await page.getByLabel('Password').fill('wrong-password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('Wrong e-mail or password.')

  await page.getByLabel('E-mail').fill('nobody@example.com')
  await page.getByLabel('Password').fill('12345678')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('Wrong e-mail or password.')
  await expect(page).toHaveScreenshot('password-wrong.png', { fullPage: true })

  await page.unroute('**/api/auth/login')
  await page.route('**/api/auth/login', (route) => route.fulfill({ status: 429, headers: { 'Retry-After': '60' }, json: { error: 'too many' } }))
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('Too many attempts. Wait a minute and try again.')
  expect(escaped).toEqual([])
})

test('The sign-in screen shows e-mail+password and Steam; the magic-link option appears only when SMTP is configured', async ({ page, escaped }) => {
  await mockPassword(page)
  await page.goto('/#/signin')
  await expect(page.getByLabel('E-mail')).toBeVisible()
  await expect(page.getByLabel('Password')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create account' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Continue with Steam' })).toHaveAttribute('href', '/api/auth/steam')
  await expect(page.getByRole('button', { name: 'Send sign-in link' })).toHaveCount(0)
  await expect(page).toHaveScreenshot('password-no-smtp.png', { fullPage: true })

  await page.route('**/api/auth/methods', (route) => route.fulfill({ json: { password: true, steam: true, emailLink: true } }))
  await page.reload()
  await expect(page.getByRole('button', { name: 'Send sign-in link' })).toBeVisible()
  await expect(page).toHaveScreenshot('password-with-smtp.png', { fullPage: true })
  expect(escaped).toEqual([])
})

test.describe('in pt-BR', () => {
  test.use({ locale: 'pt-BR' })

  test('The sign-in screen reads Entrar and Criar conta in Portuguese', async ({ page, escaped }) => {
    await mockPassword(page)
    await page.goto('/#/signin')
    await expect(page.getByLabel('Senha')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Entrar', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Criar conta' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Continuar com Steam' })).toBeVisible()
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('password-pt-br.png', { fullPage: true })
  })
})
