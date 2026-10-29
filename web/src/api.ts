export type Health = { status: string; database: string }

export type Hero = {
  id: number
  slug: string
  name: string
  primaryAttr: string
  attackType: string
  roles: string[]
  img: string
  icon: string
}

export type HeroCatalog = { patch: string; heroes: Hero[] }

// Hero image paths from dotaconstants are relative to Valve's CDN.
export const heroImageBase = 'https://cdn.cloudflare.steamstatic.com'

async function getJSON<T>(path: string, signal?: AbortSignal, okStatuses: number[] = []): Promise<T> {
  const res = await fetch(path, { signal })
  if (!res.ok && !okStatuses.includes(res.status)) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as T
}

export const fetchHealth = (signal?: AbortSignal) => getJSON<Health>('/api/health', signal, [503])

export const fetchHeroes = (signal?: AbortSignal) => getJSON<HeroCatalog>('/api/heroes', signal)

export type HeroStats = {
  strBase: number
  strGain: number
  agiBase: number
  agiGain: number
  intBase: number
  intGain: number
  armor: number
  magicResistance: number
  damageMin: number
  damageMax: number
  attackRate: number
  attackRange: number
  movementSpeed: number
  maxHealth: number
  healthRegen: number
  maxMana: number
  manaRegen: number
}

export type Ability = {
  id: number
  kind: 'ability' | 'innate'
  name: string
  description: string
  lore: string
  cooldowns: number[]
  manaCosts: number[]
}

export type Talent = { id: number; level: number; name: string }

// A hero's page on the current patch; stats is null until its details are synced.
export type HeroDetail = Hero & {
  patch: string
  lang: string
  hype: string
  complexity: number
  stats: HeroStats | null
  abilities: Ability[]
  talents: Talent[]
}

// The hero with its text in lang, or null when the current patch has no such hero.
export async function fetchHero(slug: string, lang: string, signal?: AbortSignal): Promise<HeroDetail | null> {
  const path = `/api/heroes/${encodeURIComponent(slug)}?lang=${encodeURIComponent(lang)}`
  const res = await fetch(path, { signal })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as HeroDetail
}

// A draft as POST /api/draft/suggest takes it: hero ids per side and the
// player's position, 1 to 5.
export type DraftInput = { allies: number[]; enemies: number[]; bans: number[]; role: number }

// A candidate's shrunk win rate against one enemy minus 50, in points.
export type EnemyDelta = { heroId: number; delta: number; games: number }

export type Suggestion = {
  heroId: number
  score: number
  counter: number
  winRate: number
  roleFit: number
  enemies: EnemyDelta[]
  // Jev's probability, when Jev chose among the top of the ranking.
  probability?: number
}

// Why the code ranking was kept instead of Jev's order.
export type FallbackReason = 'timeout' | 'error' | 'unavailable' | 'few_candidates'

// source is 'jev' when Jev ordered the list, else the code ranking stands
// with its reason (ADR-0003: fail open).
export type SuggestResult = {
  patch: string
  source: 'jev' | 'ranking'
  reason?: FallbackReason
  confidence?: number
  suggestions: Suggestion[]
}

export async function fetchSuggestions(input: DraftInput, signal?: AbortSignal): Promise<SuggestResult> {
  const path = '/api/draft/suggest'
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  })
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  const body = (await res.json()) as Partial<SuggestResult>
  return {
    patch: body.patch ?? '',
    source: body.source === 'jev' ? 'jev' : 'ranking',
    reason: body.reason,
    confidence: body.confidence,
    suggestions: body.suggestions ?? [],
  }
}

// One slot of a draft read from a photo: a hero of the current patch, an
// empty slot, or one the reader could not read, which the player picks.
export type ReadSlot = { heroId?: number; status: 'hero' | 'empty' | 'unknown'; confidence: number }

// The draft POST /api/draft/read read, five slots a team, left to right.
export type DraftRead = { patch: string; reader: string; fallback?: string; radiant: ReadSlot[]; dire: ReadSlot[] }

const asSlots = (v: unknown): ReadSlot[] =>
  Array.from({ length: 5 }, (_, i) => {
    const s = Array.isArray(v) ? (v[i] as Partial<ReadSlot> | undefined) : undefined
    return s?.status === 'hero' && typeof s.heroId === 'number'
      ? { heroId: s.heroId, status: 'hero', confidence: s.confidence ?? 0 }
      : { status: s?.status === 'empty' ? 'empty' : 'unknown', confidence: s?.confidence ?? 0 }
  })

// Sends the photo to be read; it is not kept by the server.
export async function readDraft(image: Blob, signal?: AbortSignal): Promise<DraftRead> {
  const path = '/api/draft/read'
  const form = new FormData()
  form.append('image', image, 'pick.jpg')
  const res = await fetch(path, { method: 'POST', body: form, signal })
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  const body = (await res.json()) as Partial<DraftRead>
  return { patch: body.patch ?? '', reader: body.reader ?? '', fallback: body.fallback, radiant: asSlots(body.radiant), dire: asSlots(body.dire) }
}
