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
export type FallbackReason = 'timeout' | 'error' | 'unavailable' | 'few_candidates' | 'low_confidence'

// source is 'jev' when Jev ordered the list, else the code ranking stands
// with its reason (ADR-0003: fail open).
export type SuggestResult = {
  patch: string
  source: 'jev' | 'ranking'
  reason?: FallbackReason
  confidence?: number
  suggestions: Suggestion[]
}

// A top hero's tip: the writer's text, or a template the UI words from the
// hero's reasons when the text failed the entity check, was late or failed.
export type Tip = { heroId: number; source: 'writer' | 'template'; text?: string; reason?: string }

const asSuggestions = (v: unknown): Suggestion[] => (Array.isArray(v) ? (v as Suggestion[]) : [])

function asDecision(body: Partial<SuggestResult>): Omit<SuggestResult, 'patch'> {
  return {
    source: body.source === 'jev' ? 'jev' : 'ranking',
    reason: body.reason,
    confidence: body.confidence,
    suggestions: asSuggestions(body.suggestions),
  }
}

// One slot of a draft read from a photo: a hero of the current patch, an
// empty slot, or one the reader could not read, which the player picks.
export type ReadSlot = { heroId?: number; status: 'hero' | 'empty' | 'unknown'; confidence: number }

// The draft read from a photo, five slots a team, left to right.
export type DraftRead = { patch: string; reader: string; fallback?: string; radiant: ReadSlot[]; dire: ReadSlot[] }

const asSlots = (v: unknown): ReadSlot[] =>
  Array.from({ length: 5 }, (_, i) => {
    const s = Array.isArray(v) ? (v[i] as Partial<ReadSlot> | undefined) : undefined
    return s?.status === 'hero' && typeof s.heroId === 'number'
      ? { heroId: s.heroId, status: 'hero', confidence: s.confidence ?? 0 }
      : { status: s?.status === 'empty' ? 'empty' : 'unknown', confidence: s?.confidence ?? 0 }
  })

const asRead = (body: Partial<DraftRead>): DraftRead => ({
  patch: body.patch ?? '',
  reader: body.reader ?? '',
  fallback: body.fallback,
  radiant: asSlots(body.radiant),
  dire: asSlots(body.dire),
})

// The draft a photo shows, with the draft the server seated the player in.
export type StreamRead = DraftRead & { draft: DraftInput }

export type StreamEvent =
  | { type: 'read'; read: StreamRead }
  | { type: 'ranking'; patch: string; suggestions: Suggestion[] }
  | { type: 'decision'; decision: Omit<SuggestResult, 'patch'> }
  | { type: 'tip'; tip: Tip }
  | { type: 'done' }

// What POST /api/draft/stream ranks: a draft, or a photo with the player's
// team and position. lang is the tips' language.
export type StreamRequest =
  | { kind: 'draft'; draft: DraftInput; lang: string }
  | { kind: 'photo'; image: Blob; team: 'radiant' | 'dire'; role: number; lang: string }

const ids = (v: unknown) => (Array.isArray(v) ? (v as number[]) : [])

function asEvent(name: string, data: string): StreamEvent | null {
  let body: Record<string, unknown>
  try {
    body = JSON.parse(data) as Record<string, unknown>
  } catch {
    return null
  }
  switch (name) {
    case 'read': {
      const d = (body.draft ?? {}) as Partial<DraftInput>
      const draft = { allies: ids(d.allies), enemies: ids(d.enemies), bans: ids(d.bans), role: d.role ?? 0 }
      return { type: 'read', read: { ...asRead(body as Partial<DraftRead>), draft } }
    }
    case 'ranking':
      return { type: 'ranking', patch: String(body.patch ?? ''), suggestions: asSuggestions(body.suggestions) }
    case 'decision':
      return { type: 'decision', decision: asDecision(body as Partial<SuggestResult>) }
    case 'tip': {
      if (typeof body.heroId !== 'number') return null
      const text = typeof body.text === 'string' && body.text !== '' ? body.text : undefined
      const reason = typeof body.reason === 'string' ? body.reason : undefined
      return { type: 'tip', tip: { heroId: body.heroId, source: body.source === 'writer' && text ? 'writer' : 'template', text, reason } }
    }
    case 'done':
      return { type: 'done' }
  }
  return null
}

// Streams the suggestions for req over Server-Sent Events, calling onEvent
// for each as it arrives; resolves when the stream ends.
export async function streamSuggestions(req: StreamRequest, onEvent: (e: StreamEvent) => void, signal?: AbortSignal): Promise<void> {
  const query = new URLSearchParams({ lang: req.lang })
  let init: RequestInit
  if (req.kind === 'photo') {
    query.set('role', String(req.role))
    query.set('team', req.team)
    const form = new FormData()
    form.append('image', req.image, 'pick.jpg')
    init = { method: 'POST', body: form, signal }
  } else {
    init = { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req.draft), signal }
  }
  const res = await fetch(`/api/draft/stream?${query}`, init)
  if (!res.ok || !res.body) throw new Error(`/api/draft/stream: HTTP ${res.status}`)
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    buf += value.replace(/\r\n?/g, '\n')
    for (let end = buf.indexOf('\n\n'); end >= 0; end = buf.indexOf('\n\n')) {
      const block = buf.slice(0, end)
      buf = buf.slice(end + 2)
      let name = 'message'
      const data: string[] = []
      for (const line of block.split('\n')) {
        if (line.startsWith('event:')) name = line.slice(6).trim()
        else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
      }
      const ev = asEvent(name, data.join('\n'))
      if (ev) onEvent(ev)
    }
  }
}

// One line of a patch's notes; ability names the ability when the current
// patch has it.
export type PatchNote = { kind: 'hero' | 'ability' | 'talent' | 'item' | 'general'; ability?: string; indent: number; text: string }

// A hero's notes per patch, newest patch first.
export type HeroHistory = { slug: string; lang: string; patches: { name: string; releasedAt: string; notes: PatchNote[] }[] }

export type PatchSummary = { name: string; releasedAt: string; heroes: number; items: number }

// What one patch changed, in the requested language when the patch has it.
export type PatchChanges = {
  name: string
  releasedAt: string
  lang: string
  general: PatchNote[]
  heroes: (Hero & { notes: PatchNote[] })[]
  items: { id: number; name: string; notes: PatchNote[] }[]
}

// The hero's history, or null when no hero has that slug.
export async function fetchHeroHistory(slug: string, lang: string, signal?: AbortSignal): Promise<HeroHistory | null> {
  const path = `/api/heroes/${encodeURIComponent(slug)}/history?lang=${encodeURIComponent(lang)}`
  const res = await fetch(path, { signal })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as HeroHistory
}

export const fetchPatches = (signal?: AbortSignal) => getJSON<{ patches: PatchSummary[] }>('/api/patches', signal)

export const fetchPatch = (name: string, lang: string, signal?: AbortSignal) =>
  getJSON<PatchChanges>(`/api/patches/${encodeURIComponent(name)}?lang=${encodeURIComponent(lang)}`, signal)

// A hero's record against one opponent on the current patch: its shrunk win
// rate minus 50, in points; lowSample under lowSampleGames games.
export type Counter = Hero & { delta: number; games: number; lowSample: boolean }

export type Counters = { patch: string; lowSampleGames: number; goodAgainst: Counter[]; badAgainst: Counter[] }

// The hero's counters, or null when the current patch has no such hero.
export async function fetchCounters(slug: string, signal?: AbortSignal): Promise<Counters | null> {
  const path = `/api/heroes/${encodeURIComponent(slug)}/counters`
  const res = await fetch(path, { signal })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as Counters
}
