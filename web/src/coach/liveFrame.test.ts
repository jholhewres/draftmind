import { expect, test } from 'vitest'
import { grid, inGame, moved } from './liveFrame'

// A 64x36 frame: the world at brightness world, the HUD band at hud.
function frame(world: number, hud: number, spot = 0) {
  const width = 64
  const height = 36
  const data = new Uint8ClampedArray(width * height * 4)
  for (let y = 0; y < height; y++)
    for (let x = 0; x < width; x++) {
      const v = y >= height - 5 ? hud : x < 16 && y < 9 ? world + spot : world
      const i = (y * width + x) * 4
      data[i] = data[i + 1] = data[i + 2] = v
      data[i + 3] = 255
    }
  return { width, height, data }
}

test('only a changed in-game frame is worth sending', () => {
  // A loading screen and a menu without the HUD band are not a match.
  expect(inGame(grid(frame(2, 2)))).toBe(false)
  expect(inGame(grid(frame(90, 140)))).toBe(false)
  expect(inGame(grid(frame(110, 20)))).toBe(true)
  // The same frame did not move; a big change in one corner did.
  expect(moved(grid(frame(110, 20)), grid(frame(110, 20)))).toBe(false)
  expect(moved(grid(frame(110, 20)), grid(frame(110, 20, 120)))).toBe(true)
})
