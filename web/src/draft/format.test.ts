import { formatDelta } from './format'

test('the arrow follows the rounded text, halves included', () => {
  expect(formatDelta('en', 3.06)).toEqual({ arrow: '▲', text: '+3.1' })
  expect(formatDelta('en', -0.62)).toEqual({ arrow: '▼', text: '−0.6' })
  expect(formatDelta('en', 0.04)).toEqual({ arrow: '', text: '0.0' })
  expect(formatDelta('en', -0.05)).toEqual({ arrow: '▼', text: '−0.1' })
  expect(formatDelta('en', 0.05)).toEqual({ arrow: '▲', text: '+0.1' })
  expect(formatDelta('pt-BR', -2.44)).toEqual({ arrow: '▼', text: '−2,4' })
})

test('the arrow comes from the sign, wherever a locale puts it', () => {
  expect(formatDelta('ar', -0.62).arrow).toBe('▼')
  expect(formatDelta('ar', 3.06).arrow).toBe('▲')
  expect(formatDelta('ar', 0).arrow).toBe('')
})
