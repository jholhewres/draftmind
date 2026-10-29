import { describe, expect, test } from 'vitest'
import { changed, signature, watcher, type Frame } from './frameDiff'

// A 160x90 frame of the pick screen as the sampler sees it: a dark top bar,
// the clock at its centre, and a portrait colour in each locked slot.
function frame(locked: Record<number, number> = {}, clock = 0): Frame {
  const width = 160
  const height = 90
  const data = new Uint8ClampedArray(width * height * 4)
  const paint = (x0: number, x1: number, y1: number, v: number) => {
    for (let y = 0; y < y1; y++)
      for (let x = x0; x < x1; x++) {
        const i = (y * width + x) * 4
        data[i] = data[i + 1] = data[i + 2] = v
        data[i + 3] = 255
      }
  }
  paint(0, width, height, 20)
  // The clock ticks every second: it must never count as a change.
  paint(76, 84, 6, 40 + clock * 30)
  // Slots 0-4 radiant from x 34, 5-9 dire from x 90, 8 px wide each.
  for (const [slot, v] of Object.entries(locked)) {
    const s = Number(slot)
    const x0 = s < 5 ? 34 + s * 8 : 90 + (s - 5) * 8
    paint(x0, x0 + 7, 6, v)
  }
  return { width, height, data }
}

describe('frameDiff', () => {
  test('a locked portrait changes the signature; the ticking clock does not', () => {
    expect(changed(signature(frame()), signature(frame({}, 5)))).toBe(false)
    expect(changed(signature(frame()), signature(frame({ 2: 200 })))).toBe(true)
    expect(changed(signature(frame({ 7: 180 })), signature(frame({ 7: 180 }, 3)))).toBe(false)
  })

  test('only a frame where a slot changed, and held for two samples, is sent', () => {
    const w = watcher()
    // The first frame is the draft as it stands.
    expect(w.next(signature(frame()))).toBe(true)
    expect(w.next(signature(frame({}, 1)))).toBe(false)
    expect(w.next(signature(frame({}, 2)))).toBe(false)
    // A hero locks: sent once it holds still, then never again.
    expect(w.next(signature(frame({ 6: 160 }, 3)))).toBe(false)
    expect(w.next(signature(frame({ 6: 160 }, 4)))).toBe(true)
    expect(w.next(signature(frame({ 6: 160 }, 5)))).toBe(false)
    expect(w.next(signature(frame({ 6: 160 }, 6)))).toBe(false)
    // A flicker that does not hold is never sent.
    expect(w.next(signature(frame({ 6: 160, 1: 90 }, 7)))).toBe(false)
    expect(w.next(signature(frame({ 6: 160 }, 8)))).toBe(false)
  })
})
