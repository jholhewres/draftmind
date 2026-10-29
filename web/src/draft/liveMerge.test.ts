import { describe, expect, it } from 'vitest'
import type { ReadSlot } from '../api'
import { mergeLive } from './liveMerge'

const hero = (heroId: number, confidence = 0.9): ReadSlot => ({ heroId, status: 'hero', confidence })
const unknown: ReadSlot = { status: 'unknown', confidence: 0 }
const empty: ReadSlot = { status: 'empty', confidence: 1 }

describe('mergeLive', () => {
  it('keeps a read hero when a later read misses it', () => {
    expect(mergeLive([hero(1), empty], [unknown, empty], 'jev')).toEqual([hero(1), empty])
    expect(mergeLive([hero(1), empty], [empty, empty], 'jev')).toEqual([hero(1), empty])
  })
  it('adds the heroes a later read finds', () => {
    expect(mergeLive([hero(1), empty], [hero(1), hero(2)], 'jev')).toEqual([hero(1), hero(2)])
  })
  it('replaces a hero only when Jev names another one more surely', () => {
    expect(mergeLive([hero(1, 0.6)], [hero(2, 0.9)], 'jev')).toEqual([hero(2, 0.9)])
    expect(mergeLive([hero(1, 0.9)], [hero(2, 0.6)], 'jev')).toEqual([hero(1, 0.9)])
    expect(mergeLive([hero(1, 0.6)], [hero(2, 0.99)], 'vision')).toEqual([hero(1, 0.6)])
  })
  it('takes the first read as it is', () => {
    expect(mergeLive(null, [hero(1), unknown], 'vision')).toEqual([hero(1), unknown])
  })
})
