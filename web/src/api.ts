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
