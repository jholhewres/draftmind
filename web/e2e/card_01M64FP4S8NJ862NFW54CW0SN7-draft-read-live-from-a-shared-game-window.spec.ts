import type { Page } from '@playwright/test'
import { expect, imagesLoaded, rankFor, sse, test, type DraftInput } from './fixtures.ts'
import read from './fixtures/read.json' with { type: 'json' }

// A fake game window: a 1600x900 canvas shared through getDisplayMedia. A
// hero locking in the dire third slot is window.lock(); the constraints the
// page asked for are kept in window.asked.
async function fakeWindow(page: Page) {
  await page.addInitScript(() => {
    const w = window as unknown as { lock: () => void; asked: unknown[] }
    w.asked = []
    let locked = false
    w.lock = () => {
      locked = true
    }
    navigator.mediaDevices.getDisplayMedia = async (constraints?: DisplayMediaStreamOptions) => {
      w.asked.push(constraints)
      const c = document.createElement('canvas')
      c.width = 1600
      c.height = 900
      const ctx = c.getContext('2d')!
      const paint = () => {
        ctx.fillStyle = '#141414'
        ctx.fillRect(0, 0, 1600, 900)
        if (locked) {
          ctx.fillStyle = '#d0a060'
          ctx.fillRect(1056, 0, 80, 63)
        }
      }
      paint()
      window.setInterval(paint, 200)
      const stream = c.captureStream(5)
      const track = stream.getVideoTracks()[0]
      Object.defineProperty(track, 'label', { value: 'Dota 2' })
      track.getSettings = () => ({ displaySurface: 'window' }) as MediaTrackSettings
      return stream
    }
  })
}

// Each frame upload is read: the first with one dire slot unread, the
// second with it locked as Drow Ranger. Each read costs $0.002.
async function mockLiveReads(page: Page, drafts: DraftInput[]) {
  const reads: number[] = []
  await page.route('**/api/draft/stream**', async (route) => {
    if (!route.request().headers()['content-type']?.startsWith('multipart/form-data')) return route.fallback()
    reads.push(reads.length)
    const second = reads.length > 1
    const dire = second ? read.dire.map((s, i) => (i === 2 ? { heroId: 6, status: 'hero', confidence: 0.9 } : s)) : read.dire
    const seated = { allies: [5, 3], enemies: second ? [2, 7, 6] : [2, 7], bans: [], role: 1 }
    drafts.push(seated)
    const suggestions = rankFor(seated)
    await route.fulfill(
      sse([
        ['read', { ...read, dire, draft: seated, cost: 0.002 }],
        ['ranking', { patch: read.patch, suggestions }],
        ['decision', { source: 'jev', confidence: 0.4, suggestions }],
        ['done', {}],
      ]),
    )
  })
  return reads
}

test('"Live from screen" is offered next to photo and typing on desktop browsers that support getDisplayMedia, and is hidden elsewhere; the share asks for a window, and a "Live" indicator and a Stop button are always visible', async ({
  page,
  drafts,
}, testInfo) => {
  await fakeWindow(page)
  const reads = await mockLiveReads(page, drafts)
  await page.goto('/#/draft')
  const start = page.getByRole('button', { name: /Live from screen/ })
  await expect(page.getByText('From the gallery')).toBeVisible()
  await expect(page.getByText('Build by hand')).toBeVisible()
  if (testInfo.project.name === 'phone') {
    await expect(start).toHaveCount(0)
    await expect(page).toHaveScreenshot('live-hidden-phone.png')
    return
  }
  await start.click()
  expect(await page.evaluate(() => (window as unknown as { asked: { video: { displaySurface: string } }[] }).asked[0].video.displaySurface)).toBe('window')

  // The first frame is read once; the draft shows with its unread slot.
  const bar = page.getByRole('region', { name: 'Live read' })
  await expect(bar.getByText('Live', { exact: true })).toBeVisible()
  await expect(bar.getByRole('button', { name: 'Stop sharing' })).toBeVisible()
  await expect(page.getByRole('group', { name: 'Enemy' }).getByRole('button', { name: 'Unread hero, choose' })).toBeVisible()
  await expect(bar).toContainText('Watching "Dota 2" · 1 reads · $0.002')

  // A slot changes in the window: one more frame, and the board and the
  // suggestions update in place.
  await page.evaluate(() => (window as unknown as { lock: () => void }).lock())
  await expect(page.getByRole('group', { name: 'Enemy' }).getByRole('button', { name: 'Enemy: Drow Ranger' })).toBeVisible({ timeout: 10_000 })
  await expect(bar).toContainText('2 reads · $0.004')
  await expect.poll(() => drafts.at(-1)).toEqual({ allies: [5, 3], enemies: [2, 7, 6], bans: [], role: 1 })
  // An unchanged window sends nothing more.
  await page.waitForTimeout(4500)
  expect(reads).toHaveLength(2)

  await expect(page.locator('footer')).toContainText('never touches the game client')
  await imagesLoaded(page.locator('main'))
  await expect(page).toHaveScreenshot('live-draft.png')

  await bar.getByRole('button', { name: 'Stop sharing' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Photograph the pick')
})
