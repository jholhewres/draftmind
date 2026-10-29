import type { ReadSlot } from '../api'

// Merges a new live read into the slots already on screen. A draft only
// grows: a hero once read stays unless the Jev reader names another one
// more surely, and a slot the new read could not see (unread, empty, or a
// fallback read that flags everything) keeps what was there.
export function mergeLive(prev: ReadSlot[] | null, next: ReadSlot[], reader: string): ReadSlot[] {
  if (!prev || prev.length !== next.length) return next
  return next.map((n, i) => {
    const p = prev[i]
    if (p.status !== 'hero') return n.status === 'empty' && p.status === 'unknown' ? p : n
    if (n.status !== 'hero') return p
    if (n.heroId === p.heroId) return n.confidence > p.confidence ? n : p
    return reader === 'jev' && n.confidence > p.confidence ? n : p
  })
}

// Whether a live read found a draft at all: a frame of the menus or the
// match search reads as ten empty or unread slots, and is dropped.
export const hasDraft = (slots: ReadSlot[]) => slots.some((s) => s.status === 'hero')
