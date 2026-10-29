import type { Page } from '@playwright/test'
import { expect, imagesLoaded, sse, test } from './fixtures.ts'
import session from './fixtures/match.json' with { type: 'json' }

type Spoken = { text: string; lang: string }

// Stands in for the microphone, MediaRecorder and speechSynthesis: the
// recorder hands over one second of a 440 Hz tone as a 48 kHz WAV, and
// every utterance is kept in window.__spoken.
async function fakeAudio(page: Page, { denied }: { denied: boolean }) {
  await page.addInitScript(
    ({ denied, id }) => {
      window.localStorage.setItem('draftmind.match', id)
      const w = window as unknown as Record<string, unknown>
      const spoken: Spoken[] = []
      w.__spoken = spoken
      const tone = () => {
        const rate = 48000
        const n = rate
        const v = new DataView(new ArrayBuffer(44 + n * 2))
        const text = (at: number, s: string) => [...s].forEach((c, i) => v.setUint8(at + i, c.charCodeAt(0)))
        text(0, 'RIFF')
        v.setUint32(4, 36 + n * 2, true)
        text(8, 'WAVEfmt ')
        v.setUint32(16, 16, true)
        v.setUint16(20, 1, true)
        v.setUint16(22, 1, true)
        v.setUint32(24, rate, true)
        v.setUint32(28, rate * 2, true)
        v.setUint16(32, 2, true)
        v.setUint16(34, 16, true)
        text(36, 'data')
        v.setUint32(40, n * 2, true)
        for (let i = 0; i < n; i++) v.setInt16(44 + i * 2, Math.round(Math.sin((2 * Math.PI * 440 * i) / rate) * 8000), true)
        return new Blob([v], { type: 'audio/wav' })
      }
      Object.defineProperty(navigator, 'mediaDevices', {
        configurable: true,
        value: {
          getUserMedia: () =>
            denied
              ? Promise.reject(new DOMException('Permission denied', 'NotAllowedError'))
              : Promise.resolve({ getTracks: () => [{ stop() {} }] }),
        },
      })
      class FakeRecorder {
        state = 'inactive'
        mimeType = 'audio/wav'
        ondataavailable: ((e: { data: Blob }) => void) | null = null
        onstop: (() => void) | null = null
        start() {
          this.state = 'recording'
        }
        stop() {
          this.state = 'inactive'
          this.ondataavailable?.({ data: tone() })
          this.onstop?.()
        }
      }
      w.MediaRecorder = FakeRecorder
      class FakeUtterance {
        text: string
        lang = ''
        constructor(text: string) {
          this.text = text
        }
      }
      w.SpeechSynthesisUtterance = FakeUtterance
      Object.defineProperty(window, 'speechSynthesis', {
        configurable: true,
        value: { speak: (u: FakeUtterance) => spoken.push({ text: u.text, lang: u.lang }), cancel() {} },
      })
    },
    { denied, id: session.id },
  )
}

const transcript = 'pudge keeps hooking me mid'
const reply = 'Stay behind your creeps and keep Blink Dagger ready for the hook.'

type Clip = { contentType: string; head: string; rate: number; channels: number }

// The match server: the voice route takes the clip, answers with the
// transcript and a coach turn, and text messages get a plain answer.
async function matchServer(page: Page) {
  const clips: Clip[] = []
  const texts: string[] = []
  const stored: unknown[] = []
  const answer = (text: string) => ['answer', { kind: 'answer', category: 'lane', priority: 'high', source: 'jev', text, textSource: 'writer' }] as [string, unknown]
  const turn = (mine: Record<string, unknown>, text: string) => {
    stored.push(mine, { id: stored.length + 2, author: 'coach', kind: 'answer', text, category: 'lane', priority: 'high' })
    return sse([['message', mine], ['decision', { category: 'lane', priority: 'high', source: 'jev' }], answer(text), ['done', {}]])
  }
  await page.route(`**/api/match/${session.id}`, (route) => route.fulfill({ json: { ...session, messages: stored } }))
  await page.route(`**/api/match/${session.id}/voice**`, (route) => {
    const body = route.request().postDataBuffer()!
    clips.push({
      contentType: route.request().headers()['content-type'],
      head: body.subarray(0, 4).toString('latin1') + body.subarray(8, 12).toString('latin1'),
      rate: body.readUInt32LE(24),
      channels: body.readUInt16LE(22),
    })
    return route.fulfill(turn({ id: stored.length + 1, author: 'player', kind: 'voice', text: transcript }, reply))
  })
  await page.route(`**/api/match/${session.id}/messages**`, (route) => {
    const { text } = route.request().postDataJSON() as { text: string }
    texts.push(text)
    return route.fulfill(turn({ id: stored.length + 1, author: 'player', kind: 'text', text }, 'Ward the river rune.'))
  })
  return { clips, texts }
}

const chat = (page: Page) => page.getByRole('region', { name: 'Conversation' })

async function holdToTalk(page: Page) {
  const button = page.getByRole('button', { name: 'Hold to talk' })
  await button.hover()
  await page.mouse.down()
  await expect(button).toHaveAttribute('aria-pressed', 'true')
  await page.waitForTimeout(600)
  await page.mouse.up()
}

test('A recorded clip is transcribed and answered through the same coach pipeline as text', async ({ page, escaped }) => {
  await fakeAudio(page, { denied: false })
  const server = await matchServer(page)
  await page.goto('/#/coach')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Juggernaut · Carry')

  await holdToTalk(page)
  await expect(chat(page).getByRole('listitem').nth(1)).toContainText(reply)
  // One clip went up, as the 16 kHz mono WAV the server takes.
  expect(server.clips).toEqual([{ contentType: 'audio/wav', head: 'RIFFWAVE', rate: 16000, channels: 1 }])
  // The answer is read aloud in the UI language.
  await expect.poll(() => page.evaluate(() => (window as unknown as { __spoken: Spoken[] }).__spoken)).toEqual([{ text: reply, lang: 'en' }])
  expect(escaped).toEqual([])
})

test('The transcript shows with the answer', async ({ page }) => {
  await fakeAudio(page, { denied: false })
  await matchServer(page)
  await page.goto('/#/coach')

  await holdToTalk(page)
  await expect(chat(page).getByRole('listitem')).toHaveText([`Voice${transcript}`, /^Lane · high priority\s*Stay behind your creeps.*Listen$/])

  // The transcript stays next to its answer after a reload.
  await page.reload()
  await expect(chat(page).getByRole('listitem')).toHaveText([`Voice${transcript}`, /^Lane · high priority\s*Stay behind your creeps.*Listen$/])
  await imagesLoaded(page.locator('.coach-head'))
  await expect(page).toHaveScreenshot('voice-transcript.png', { fullPage: true })
})

test('A denied microphone permission shows a message and keeps text input working', async ({ page }) => {
  await fakeAudio(page, { denied: true })
  const server = await matchServer(page)
  await page.goto('/#/coach')

  const button = page.getByRole('button', { name: 'Hold to talk' })
  await button.hover()
  await page.mouse.down()
  await page.mouse.up()
  await expect(page.getByRole('alert')).toHaveText(
    'Microphone access is blocked. You can still type your message, or allow the microphone in the browser settings.',
  )
  expect(server.clips).toEqual([])

  await page.getByRole('textbox', { name: 'Message to the coach' }).fill('pudge is roaming top')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(chat(page).getByRole('listitem')).toHaveText(['pudge is roaming top', /Ward the river rune\./])
  expect(server.texts).toEqual(['pudge is roaming top'])
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Conversation' })).toHaveScreenshot('mic-denied.png')
})
