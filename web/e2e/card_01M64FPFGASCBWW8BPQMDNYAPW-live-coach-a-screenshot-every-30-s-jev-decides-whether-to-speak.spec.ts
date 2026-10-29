import type { Page } from '@playwright/test'
import { expect, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

// A fake game window mid-match: a bright world above the dark HUD band.
// window.scene(n) repaints the world so the next tick sees a change.
async function fakeMatchWindow(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem('draftmind.liveTick', '10')
    const w = window as unknown as { scene: (n: number) => void }
    let world = 120
    w.scene = (n: number) => {
      world = n
    }
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = document.createElement('canvas')
      c.width = 1280
      c.height = 720
      const ctx = c.getContext('2d')!
      window.setInterval(() => {
        ctx.fillStyle = `rgb(${world},${world},${world})`
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
}

// The fixture match; the first live tick stays silent, the next speaks an
// item timing with Battle Fury's card. Each tick costs $0.0012.
async function liveServer(page: Page) {
  await page.addInitScript((id) => window.localStorage.setItem('draftmind.match', id), session.id)
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: [] } }))
  const ticks: number[] = []
  await page.route(`**/api/match/${session.id}/live**`, (route) => {
    ticks.push(ticks.length)
    if (ticks.length === 1) return route.fulfill({ json: { speak: false, reason: 'silent', cost: 0.0012 } })
    return route.fulfill({
      json: {
        speak: true,
        event: 'item_timing',
        cost: 0.0012,
        message: {
          id: 500, author: 'coach', kind: 'answer', text: '', category: 'items', priority: 'high', source: 'live', reason: 'live:item_timing',
          items: [{ id: 145, key: 'bfury', name: 'Battle Fury', cost: 4100, img: '/apps/dota2/images/dota_react/items/bfury.png' }],
        },
      },
    })
  })
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const { text } = route.request().postDataJSON() as { text: string }
    return route.fulfill(
      sse([
        ['message', { id: 501, author: 'player', kind: 'text', text }],
        ['answer', { kind: 'answer', category: 'lane', priority: 'medium', source: 'jev', textSource: 'writer', text: 'Hold the lane until your team is back.' }],
        ['done', {}],
      ]),
    )
  })
  return ticks
}

test('`speak` posts one live-tagged coach message in the existing chat, written only from DB candidates (entity check), with an item card when an item is advised; cooldown and dedupe stop repeats', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await fakeMatchWindow(page)
  const ticks = await liveServer(page)
  await page.goto('/#/coach')
  const bar = page.getByRole('region', { name: 'Live coach' })
  if (testInfo.project.name === 'phone') {
    // A phone cannot share a window: no live bar.
    await expect(page.getByRole('region', { name: 'Conversation' })).toBeVisible()
    await expect(bar).toHaveCount(0)
    return
  }
  await bar.getByRole('button', { name: 'Share game window' }).click()
  await expect(bar.getByText('Live', { exact: true })).toBeVisible()

  // First tick: silent, nothing posted; the scene changes and the next tick speaks.
  await expect(bar).toContainText('1 looks · 0 spoken', { timeout: 15_000 })
  await expect(page.locator('.live-line-tag')).toHaveCount(0)
  await page.evaluate(() => (window as unknown as { scene: (n: number) => void }).scene(170))
  const line = page.locator('.bubble.coach').filter({ has: page.locator('.live-line-tag') })
  await expect(line).toContainText('You have the gold: buy Battle Fury now.', { timeout: 15_000 })
  await expect(line.getByRole('list', { name: 'Items in this answer' })).toContainText('Battle Fury')
  await expect(bar).toContainText('2 looks · 1 spoken · $0.002')

  // An unchanged window sends nothing more.
  await page.waitForTimeout(11_000)
  expect(ticks).toHaveLength(2)

  // The chat keeps answering manual questions during live mode.
  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('They are pushing mid')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Hold the lane until your team is back.')).toBeVisible()
  await expect(page).toHaveScreenshot('live-coach.png')

  await bar.getByRole('button', { name: 'Stop sharing' }).click()
  await expect(bar.getByRole('button', { name: 'Share game window' })).toBeVisible()
})
