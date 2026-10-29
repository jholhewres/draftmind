// The change detector for the live read: it runs in the browser on a small
// sample of each shared frame, so only a frame where a hero slot changed is
// sent to the reader (ADR-0013). It never reads anything but pixels.

export type Frame = { width: number; height: number; data: Uint8ClampedArray }

// Where the ten portraits sit on the 16:9 pick screen, as a share of the
// frame: the top bar, radiant left of the clock, dire right of it. The clock
// between them ticks every second and is left out.
const TOP = 0.07
const SIDES = [
  [0.21, 0.46],
  [0.56, 0.81],
] as const
const SLOTS = 5

// How far a slot's mean colour must move, on 0-255, to count as a change.
export const THRESHOLD = 12

// The mean RGB of each of the ten slots, radiant 0-4 then dire 5-9.
export function signature(f: Frame): number[] {
  const out: number[] = []
  const y1 = Math.max(1, Math.round(f.height * TOP))
  for (const [a, b] of SIDES) {
    const w = (b - a) / SLOTS
    for (let s = 0; s < SLOTS; s++) {
      const x0 = Math.round(f.width * (a + s * w))
      const x1 = Math.max(x0 + 1, Math.round(f.width * (a + (s + 1) * w)))
      const sum = [0, 0, 0]
      for (let y = 0; y < y1; y++)
        for (let x = x0; x < x1; x++) {
          const i = (y * f.width + x) * 4
          sum[0] += f.data[i]
          sum[1] += f.data[i + 1]
          sum[2] += f.data[i + 2]
        }
      const n = (x1 - x0) * y1
      out.push(sum[0] / n, sum[1] / n, sum[2] / n)
    }
  }
  return out
}

// Whether any slot moved past THRESHOLD between two signatures.
export const changed = (a: number[], b: number[]) => a.some((v, i) => Math.abs(v - b[i]) > THRESHOLD)

// Decides, sample by sample, which frames to send: the first one, then one
// where a slot changed and held for two samples in a row (a portrait still
// fading in or a hover flicker is not sent).
export function watcher() {
  let sent: number[] | null = null
  let pending: number[] | null = null
  return {
    next(sig: number[]): boolean {
      if (!sent || (pending && !changed(sig, pending) && changed(sig, sent))) {
        sent = sig
        pending = null
        return true
      }
      pending = changed(sig, sent) ? sig : null
      return false
    },
  }
}
