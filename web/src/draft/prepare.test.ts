import { fitWithin } from './prepare'

test('a phone photo is scaled to 1600 px on its longest side, keeping its shape; a small one is left alone', () => {
  expect(fitWithin(4032, 3024)).toEqual({ width: 1600, height: 1200 })
  expect(fitWithin(3024, 4032)).toEqual({ width: 1200, height: 1600 })
  expect(fitWithin(1280, 720)).toEqual({ width: 1280, height: 720 })
})
