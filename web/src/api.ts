// GET /api/sync/status: the current patch and when each source last synced.
export type SyncStatus = {
  patch: string
  updatedAt: string | null
  sources: { source: string; lastSuccess: string }[]
  lastFailure: { source: string; at: string } | null
}

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

export const fetchSyncStatus = (signal?: AbortSignal) => getJSON<SyncStatus>('/api/sync/status', signal)

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
  await readEvents(res.body, (name, data) => {
    const ev = asEvent(name, data)
    if (ev) onEvent(ev)
  })
}

// Reads a Server-Sent Events body, calling onEvent with each event's name
// and data; resolves when the body ends.
async function readEvents(body: ReadableStream<BufferSource>, onEvent: (name: string, data: string) => void): Promise<void> {
  const reader = body.pipeThrough(new TextDecoderStream()).getReader()
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
      onEvent(name, data.join('\n'))
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

// An item of the current patch, with the games it was bought in.
export type BuildItem = { id: number; key: string; name: string; cost: number; count: number }

export type Phase = 'start' | 'early' | 'mid' | 'late'

export type PhaseItems = { phase: Phase; items: BuildItem[] }

// A hero's most bought items by phase on the current patch, from OpenDota
// item popularity.
export type HeroBuild = { patch: string; source: string; core: PhaseItems[] }

// The hero's popular build, or null when the current patch has no such hero.
export async function fetchHeroBuild(slug: string, signal?: AbortSignal): Promise<HeroBuild | null> {
  const path = `/api/heroes/${encodeURIComponent(slug)}/build`
  const res = await fetch(path, { signal })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as HeroBuild
}

export type Urgency = 'later' | 'soon' | 'now'

// An item for this lineup and the enemy it answers; urgency only when Jev chose it.
export type SituationalItem = { item: Omit<BuildItem, 'count'>; enemyId: number; urgency?: Urgency; score: number }

// The locked hero's build against a lineup (POST /api/draft/build).
export type Build = {
  patch: string
  popularitySource: string
  heroId: number
  core: PhaseItems[]
  situational: SituationalItem[]
  source: 'jev' | 'ranking'
  reason?: string
}

export async function fetchBuild(hero: number, enemies: number[], signal?: AbortSignal): Promise<Build> {
  const res = await fetch('/api/draft/build', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ hero, enemies }),
    signal,
  })
  if (!res.ok) throw new Error(`/api/draft/build: HTTP ${res.status}`)
  return (await res.json()) as Build
}

export type Category = 'lane' | 'teamfight' | 'items' | 'objective' | 'other'
export type Priority = 'low' | 'medium' | 'high'

// One chat line. A coach line with no text is a template the UI words: a
// follow-up question, or an answer the server could not write or check.
export type MatchMessage = {
  id: number
  author: 'player' | 'coach'
  kind: 'text' | 'voice' | 'answer' | 'followup'
  text: string
  category?: Category
  priority?: Priority
  source?: string
  reason?: string
}

// A match being coached: the draft, the locked hero, its item plan and the chat.
export type MatchSession = {
  id: string
  patch: string
  heroId: number
  role: number
  allies: number[]
  enemies: number[]
  bans: number[]
  itemPlan: Omit<Build, 'patch' | 'popularitySource'>
  messages: MatchMessage[]
  result?: MatchResult
}

export type Outcome = 'win' | 'loss'

// How a match ended, as the player tells it before confirming.
export type ResultFill = { result?: Outcome; durationS?: number; kills?: number; deaths?: number; assists?: number }

// The result the player confirmed, with the coach's notes.
export type MatchResult = ResultFill & { result: Outcome; source: 'tap' | 'message' | 'photo'; notes: string[]; endedAt: string }

// One ended match in the player's history.
export type PastMatch = ResultFill & { id: string; heroId: number; role: number; result: Outcome; endedAt: string }

const matchPath = (id: string, rest: string) => `/api/match/${encodeURIComponent(id)}/${rest}`

// Reads a message such as "we lost" into a result; null when it does not say.
export async function parseEndMessage(id: string, text: string, signal?: AbortSignal): Promise<Outcome | null> {
  const path = matchPath(id, 'end/parse')
  const res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }), signal })
  if (res.status === 422) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return ((await res.json()) as { result: Outcome }).result
}

// Reads a photo of the end screen into the result, duration and KDA it shows.
export async function readEndScreen(id: string, image: Blob, signal?: AbortSignal): Promise<ResultFill> {
  const path = matchPath(id, 'end/read')
  const form = new FormData()
  form.append('image', image, 'end.jpg')
  const res = await fetch(path, { method: 'POST', body: form, signal })
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as ResultFill
}

// Saves the confirmed result; the answer carries the coach's notes.
export async function saveMatchResult(
  id: string,
  r: ResultFill & { result: Outcome; source: MatchResult['source'] },
  lang: string,
  persona: Persona,
): Promise<MatchResult> {
  const path = matchPath(id, `result?${coachQuery(lang, persona)}`)
  const res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(r) })
  if (!res.ok) throw new Error(`/api/match/result: HTTP ${res.status}`)
  return (await res.json()) as MatchResult
}

// The signed-in player's ended matches, newest first; null for a visitor.
export async function fetchMyMatches(signal?: AbortSignal): Promise<PastMatch[] | null> {
  const res = await fetch('/api/me/matches', { signal })
  if (res.status === 401) return null
  if (!res.ok) throw new Error(`/api/me/matches: HTTP ${res.status}`)
  return (await res.json()) as PastMatch[]
}

export type MatchStart = { hero: number; role: number; allies: number[]; enemies: number[]; bans: number[] }

export async function startMatch(req: MatchStart, signal?: AbortSignal): Promise<MatchSession> {
  const res = await fetch('/api/match', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal,
  })
  if (!res.ok) throw new Error(`/api/match: HTTP ${res.status}`)
  return (await res.json()) as MatchSession
}

// The session, or null when no session has that id.
export async function fetchMatch(id: string, signal?: AbortSignal): Promise<MatchSession | null> {
  const path = `/api/match/${encodeURIComponent(id)}`
  const res = await fetch(path, { signal })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as MatchSession
}

export type CoachEvent =
  | { type: 'message'; message: MatchMessage }
  | { type: 'decision'; category: Category; priority: Priority }
  | { type: 'reply'; message: Omit<MatchMessage, 'id'> }

// Sends the player's message and streams the coach's turn.
export async function sendMatchMessage(
  id: string,
  text: string,
  lang: string,
  persona: Persona,
  onEvent: (e: CoachEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const path = `/api/match/${encodeURIComponent(id)}/messages?${coachQuery(lang, persona)}`
  const res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }), signal })
  if (!res.ok || !res.body) throw new Error(`/api/match/messages: HTTP ${res.status}`)
  await readCoachTurn(res.body, onEvent)
}

// A voice clip the server refused: 422 when it heard no words.
export class VoiceError extends Error {
  readonly status: number
  constructor(status: number) {
    super(`/api/match/voice: HTTP ${status}`)
    this.status = status
  }
}

// Sends a push-to-talk clip (16 kHz mono WAV) and streams the coach's turn
// on its transcript; the first message event carries the transcript.
export async function sendMatchVoice(
  id: string,
  wav: Blob,
  lang: string,
  persona: Persona,
  onEvent: (e: CoachEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const path = `/api/match/${encodeURIComponent(id)}/voice?${coachQuery(lang, persona)}`
  const res = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'audio/wav' }, body: wav, signal })
  if (!res.ok || !res.body) throw new VoiceError(res.status)
  await readCoachTurn(res.body, onEvent)
}

async function readCoachTurn(body: ReadableStream<BufferSource>, onEvent: (e: CoachEvent) => void) {
  await readEvents(body, (name, data) => {
    let body: Record<string, unknown>
    try {
      body = JSON.parse(data) as Record<string, unknown>
    } catch {
      return
    }
    const str = (v: unknown) => (typeof v === 'string' ? v : undefined)
    switch (name) {
      case 'message':
        onEvent({ type: 'message', message: body as MatchMessage })
        break
      case 'decision':
        onEvent({ type: 'decision', category: body.category as Category, priority: body.priority as Priority })
        break
      case 'answer':
      case 'followup':
        onEvent({
          type: 'reply',
          message: {
            author: 'coach',
            kind: name,
            // A template's text is worded here, never taken from the stream.
            text: name === 'answer' && body.textSource === 'writer' ? (str(body.text) ?? '') : '',
            category: str(body.category) as Category | undefined,
            priority: str(body.priority) as Priority | undefined,
            reason: str(body.reason),
          },
        })
        break
    }
  })
}

// The player's own coach: how it talks, never what it decides.
export type Persona = {
  tone: 'direct' | 'encouraging' | 'analytic'
  language: 'auto' | 'en' | 'pt-BR'
  verbosity: 'short' | 'normal'
  focus: 'laning' | 'teamfights' | 'items' | 'macro'
}

export const defaultPersona: Persona = { tone: 'direct', language: 'auto', verbosity: 'normal', focus: 'laning' }

// The query a coach request carries: the UI language and the persona. The
// server uses a signed-in player's saved persona instead.
const coachQuery = (lang: string, persona: Persona) => new URLSearchParams({ lang, ...persona })

// The signed-in player's persona.
export async function fetchPersona(signal?: AbortSignal): Promise<Persona> {
  const res = await fetch('/api/me/persona', { signal })
  if (!res.ok) throw new Error(`/api/me/persona: HTTP ${res.status}`)
  return (await res.json()) as Persona
}

export async function savePersona(p: Persona): Promise<void> {
  const res = await fetch('/api/me/persona', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(p) })
  if (!res.ok) throw new Error(`/api/me/persona: HTTP ${res.status}`)
}

// The UI language the signed-in player picked, or '' before they pick one.
export async function fetchUILanguage(signal?: AbortSignal): Promise<string> {
  const res = await fetch('/api/me/language', { signal })
  if (!res.ok) throw new Error(`/api/me/language: HTTP ${res.status}`)
  return ((await res.json()) as { language: string }).language
}

export async function saveUILanguage(language: string): Promise<void> {
  const res = await fetch('/api/me/language', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ language }) })
  if (!res.ok) throw new Error(`/api/me/language: HTTP ${res.status}`)
}

// The signed-in account: an e-mail address, a SteamID or both.
export type Account = { email?: string; steamId?: string }

// The account of this browser's session, or null without one.
export async function fetchAccount(signal?: AbortSignal): Promise<Account | null> {
  const res = await fetch('/api/me', { signal })
  if (res.status === 401) return null
  if (!res.ok) throw new Error(`/api/me: HTTP ${res.status}`)
  return (await res.json()) as Account
}

const postJSON = (path: string, body: unknown) =>
  fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

// The ways in the server has on; the e-mail link needs a mailer.
export type SignInMethods = { password: boolean; steam: boolean; emailLink: boolean }

export async function fetchSignInMethods(signal?: AbortSignal): Promise<SignInMethods> {
  const res = await fetch('/api/auth/methods', { signal })
  if (!res.ok) throw new Error(`/api/auth/methods: HTTP ${res.status}`)
  return (await res.json()) as SignInMethods
}

// Why a password sign-in or register was refused: bad credentials (one
// answer for a wrong password and an unknown e-mail), too many attempts,
// or a password outside 8 to 200 characters.
export class PasswordError extends Error {
  readonly reason: 'credentials' | 'rate' | 'invalid'
  constructor(reason: PasswordError['reason'], message: string) {
    super(message)
    this.reason = reason
  }
}

async function passwordCall(path: string, email: string, password: string): Promise<Response> {
  const res = await postJSON(path, { email, password })
  if (res.status === 401) throw new PasswordError('credentials', `${path}: HTTP 401`)
  if (res.status === 429) throw new PasswordError('rate', `${path}: HTTP 429`)
  if (res.status === 400) throw new PasswordError('invalid', `${path}: HTTP 400`)
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return res
}

export async function signInWithPassword(email: string, password: string): Promise<Account> {
  return (await (await passwordCall('/api/auth/login', email, password)).json()) as Account
}

// Creates the account, then signs in. A taken e-mail gets the same answer
// from register, so the sign-in that follows is what fails.
export async function registerWithPassword(email: string, password: string): Promise<Account> {
  await passwordCall('/api/auth/register', email, password)
  return signInWithPassword(email, password)
}

// E-mails a sign-in link written in lang; resolves to the minutes it works.
export async function requestSignInLink(email: string, lang: string): Promise<number> {
  const path = '/api/auth/email'
  const res = await postJSON(path, { email, lang })
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  const body = (await res.json()) as { minutes?: number }
  return body.minutes ?? 15
}

// Spends a sign-in link's token: the account it signed in, or null when
// the link expired or was already used.
export async function openSignInLink(token: string): Promise<Account | null> {
  const path = '/api/auth/email/verify'
  const res = await postJSON(path, { token })
  if (res.status === 401) return null
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`)
  return (await res.json()) as Account
}

export async function signOut(): Promise<void> {
  const res = await fetch('/api/auth/signout', { method: 'POST' })
  if (!res.ok) throw new Error(`/api/auth/signout: HTTP ${res.status}`)
}

// Sends the browser to Steam; Steam returns it to #/signin or #/signin/failed.
export const steamSignInPath = '/api/auth/steam'
