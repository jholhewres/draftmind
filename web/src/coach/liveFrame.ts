// Frame checks for the live coach, in the browser and for free: a frame is
// sent only when the screen changed and looks like a match in progress.

import type { Frame } from '../draft/frameDiff'

// The 8x8 grid of mean brightness a frame is compared on.
export function grid(f: Frame, n = 8): number[] {
  const out: number[] = []
  for (let gy = 0; gy < n; gy++)
    for (let gx = 0; gx < n; gx++) {
      const x0 = Math.floor((gx * f.width) / n)
      const x1 = Math.max(x0 + 1, Math.floor(((gx + 1) * f.width) / n))
      const y0 = Math.floor((gy * f.height) / n)
      const y1 = Math.max(y0 + 1, Math.floor(((gy + 1) * f.height) / n))
      let sum = 0
      for (let y = y0; y < y1; y++)
        for (let x = x0; x < x1; x++) {
          const i = (y * f.width + x) * 4
          sum += f.data[i] + f.data[i + 1] + f.data[i + 2]
        }
      out.push(sum / (3 * (x1 - x0) * (y1 - y0)))
    }
  return out
}

// How much the screen must change, as the mean cell move on 0-255.
export const MIN_CHANGE = 6

// Whether two grids differ enough to be worth a look.
export const moved = (a: number[], b: number[]) => a.reduce((s, v, i) => s + Math.abs(v - b[i]), 0) / a.length >= MIN_CHANGE

// Whether a frame looks like a match in progress: not a black loading
// screen, and the HUD's dark bottom band is there under a brighter world.
export function inGame(g: number[], n = 8): boolean {
  const mean = g.reduce((s, v) => s + v, 0) / g.length
  if (mean < 12) return false
  const bottom = g.slice(n * (n - 1)).reduce((s, v) => s + v, 0) / n
  const middle = g.slice(n * 3, n * 5).reduce((s, v) => s + v, 0) / (2 * n)
  return bottom < middle
}
