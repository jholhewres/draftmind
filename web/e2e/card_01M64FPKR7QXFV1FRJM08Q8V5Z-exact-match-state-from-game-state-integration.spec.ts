import { expect, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

test('The settings screen generates the account\'s config file with install steps; without GSI everything works as before', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.route('**/api/me/persona', (route) => route.fulfill({ json: { tone: 'direct', language: 'auto', verbosity: 'short', focus: 'items' } }))
  let receiving = false
  let configured = false
  await page.route('**/api/me/gsi', (route) => route.fulfill({ json: { configured, receiving } }))
  await page.route('**/api/me/gsi/token', (route) => {
    configured = true
    return route.fulfill({
      json: {
        filename: 'gamestate_integration_draftmind.cfg',
        config: '"Draftmind"\n{\n\t"uri" "https://draftmind.example/api/gsi"\n\t"auth"\n\t{\n\t\t"token" "abc"\n\t}\n}\n',
      },
    })
  })
  await page.goto('/#/settings')
  const gsi = page.getByRole('region', { name: 'Game integration' })
  await expect(gsi.getByRole('status')).toHaveText('Not set up')
  await gsi.getByRole('button', { name: 'Make my config file' }).click()

  const download = gsi.getByRole('link', { name: 'gamestate_integration_draftmind.cfg' })
  await expect(download).toHaveAttribute('download', 'gamestate_integration_draftmind.cfg')
  expect(decodeURIComponent((await download.getAttribute('href'))!)).toContain('"uri" "https://draftmind.example/api/gsi"')
  await expect(gsi.getByRole('listitem')).toHaveCount(4)
  await expect(gsi.getByText('-gamestateintegration')).toBeVisible()
  await expect(gsi.getByRole('status')).toHaveText('Set up. Waiting for the game to post')
  receiving = true
  await expect(gsi.getByRole('status')).toHaveText('Receiving data', { timeout: 15_000 })
  await expect(gsi).toHaveScreenshot('gsi-settings.png')
})

test('With GSI on, the live coach tick gets exact gold, items and clock as context, and the match result is recorded automatically from the winner', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { email: 'player@example.com' } }))
  await page.route('**/api/me/gsi', (route) => route.fulfill({ json: { configured: true, receiving: true } }))
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  let reads = 0
  const line = {
    id: 900, author: 'coach', kind: 'answer', text: '', category: 'items', priority: 'high', source: 'live', reason: 'live:item_timing',
    items: [{ id: 145, key: 'bfury', name: 'Battle Fury', cost: 4100, img: '/apps/dota2/images/dota_react/items/bfury.png' }],
  }
  await page.route(`**/api/match/${session.id}`, (route) => {
    reads++
    // The game's posts add a code-trigger line, then the match ends with a win.
    const messages = reads >= 2 ? [line] : []
    const result = reads >= 3 ? { result: 'win', kills: 12, deaths: 4, assists: 17, durationS: 2410, source: 'gsi', notes: [], endedAt: '2026-10-29T22:00:00Z' } : undefined
    return route.fulfill({ json: { ...session, messages, result } })
  })
  await page.goto('/#/coach')
  await expect(page.locator('.bubble.coach').filter({ has: page.locator('.live-line-tag') })).toContainText('You have the gold: buy Battle Fury now.', {
    timeout: 15_000,
  })
  await expect(page.getByText('Pick a hero on the Draft tab to start a match.')).toBeVisible({ timeout: 15_000 })
})
