import path from 'node:path'
import { readFileSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { expect, imagesLoaded, rankFor, sse, test, type DraftInput } from './fixtures.ts'
import read from './fixtures/read.json' with { type: 'json' }

const fixture = (name: string) => path.join(import.meta.dirname, 'fixtures', name)
const photo = fixture('pick-photo.jpg') // 2400x1350, with EXIF (make, model, date)

// The photo's bytes as POST /api/draft/stream received them.
type Upload = { body: Buffer }

// The draft the server seats from the fixture read.
const seated: DraftInput = { allies: [5, 3], enemies: [2, 7], bans: [], role: 1 }

// Streams the read of the photo with the fixture: Crystal Maiden, you and
// Bane against Axe, Earthshaker and one slot the reader could not read;
// then the ranking of that draft. A draft sent as JSON falls through to the
// shared mock. drafts records the seated draft as ranked.
async function mockRead(page: Page, drafts: DraftInput[] = []) {
  const uploads: Upload[] = []
  await page.route('**/api/draft/stream**', async (route) => {
    if (!route.request().headers()['content-type']?.startsWith('multipart/form-data')) return route.fallback()
    uploads.push({ body: route.request().postDataBuffer()! })
    drafts.push(seated)
    const suggestions = rankFor(seated)
    await route.fulfill(
      sse([
        ['read', { ...read, draft: seated }],
        ['ranking', { patch: read.patch, suggestions }],
        ['decision', { source: 'jev', confidence: 0.4, suggestions }],
        ['done', {}],
      ]),
    )
  })
  return uploads
}

// The JPEG inside a multipart body, and its size from its frame header.
function jpegIn(body: Buffer) {
  const start = body.indexOf(Buffer.from([0xff, 0xd8, 0xff]))
  const end = body.lastIndexOf(Buffer.from([0xff, 0xd9])) + 2
  const jpeg = body.subarray(start, end)
  for (let i = 2; i < jpeg.length - 9; ) {
    const marker = jpeg[i + 1]
    if (marker >= 0xc0 && marker <= 0xc2) return { jpeg, height: jpeg.readUInt16BE(i + 5), width: jpeg.readUInt16BE(i + 7) }
    i += 2 + jpeg.readUInt16BE(i + 2)
  }
  return { jpeg, width: 0, height: 0 }
}

async function takePhoto(page: Page) {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Photograph the pick')
  await page.locator('input[capture=environment]').setInputFiles(photo)
}

test('Unknown or low-confidence slots are flagged, never guessed, and fixing one re-ranks in place', async ({ page, drafts, escaped }) => {
  await mockRead(page, drafts)
  await takePhoto(page)
  const enemy = page.getByRole('group', { name: 'Enemy' })
  const flagged = enemy.getByRole('button', { name: 'Unread hero, choose' })
  await expect(flagged).toBeVisible()
  await expect(page.getByText('1 hero was not read.')).toBeVisible()
  // The ranking renders as soon as the read returns, without the unread slot.
  await expect(page.getByRole('list', { name: 'Suggestions' }).getByRole('listitem').first()).toBeVisible()
  expect(drafts).toEqual([{ allies: [5, 3], enemies: [2, 7], bans: [], role: 1 }])
  await imagesLoaded(page.locator('main'))
  await expect(page).toHaveScreenshot('photo-read-flagged.png', { fullPage: true })

  await flagged.click()
  await page.getByRole('dialog', { name: 'Choose an enemy' }).getByRole('button', { name: 'Drow Ranger' }).click()
  await expect(enemy.getByRole('button', { name: 'Enemy: Drow Ranger' })).toBeVisible()
  await expect.poll(() => drafts.at(-1)).toEqual({ allies: [5, 3], enemies: [2, 7, 6], bans: [], role: 1 })
  await expect(page.getByText(/not read/)).toHaveCount(0)
  await expect(page.getByRole('list', { name: 'Suggestions' }).getByRole('listitem')).toHaveCount(3)
  expect(escaped).toEqual([])
  await imagesLoaded(page.locator('main'))
  await expect(page).toHaveScreenshot('photo-read-fixed.png', { fullPage: true })
})

test('The image is downscaled and stripped of EXIF in the browser and never stored', async ({ page, escaped }) => {
  const original = readFileSync(photo)
  expect(original.includes('Exif')).toBe(true)
  const uploads = await mockRead(page)
  await takePhoto(page)
  await expect(page.getByRole('group', { name: 'Your team' })).toBeVisible()
  expect(uploads).toHaveLength(1)
  const { jpeg, width, height } = jpegIn(uploads[0].body)
  expect(Math.max(width, height)).toBe(1600)
  expect(width / height).toBeCloseTo(2400 / 1350, 2)
  expect(jpeg.includes('Exif')).toBe(false)
  expect(jpeg.includes('TestPhone')).toBe(false)
  expect(jpeg.length).toBeLessThan(5 * 1024 * 1024)
  // Nothing of the photo stays in the page's storage.
  expect(await page.evaluate(() => localStorage.length + sessionStorage.length)).toBe(0)
  expect(escaped).toEqual([])
  await page.getByRole('button', { name: 'New photo' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Photograph the pick')
  await expect(page).toHaveScreenshot('photo-capture.png', { fullPage: true })
})
