import { createContext, useContext } from 'react'
import { defaultPersona, type Persona } from '../api'

// A visitor's persona lives in this browser; a signed-in player's on the server.
const key = 'draftmind.persona'

const allowed: { [F in keyof Persona]: readonly Persona[F][] } = {
  tone: ['direct', 'encouraging', 'analytic'],
  language: ['auto', 'en', 'pt-BR'],
  verbosity: ['short', 'normal'],
  focus: ['laning', 'teamfights', 'items', 'macro'],
}

const pick = <F extends keyof Persona>(raw: Record<string, unknown>, f: F): Persona[F] =>
  allowed[f].find((v) => v === raw[f]) ?? defaultPersona[f]

// The persona kept in this browser; a missing or unknown value is the default.
export function storedPersona(): Persona {
  let raw: unknown = null
  try {
    raw = JSON.parse(window.localStorage.getItem(key) ?? 'null')
  } catch {
    // nothing kept, or storage is blocked
  }
  const r = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
  return { tone: pick(r, 'tone'), language: pick(r, 'language'), verbosity: pick(r, 'verbosity'), focus: pick(r, 'focus') }
}

export function storePersona(p: Persona) {
  try {
    window.localStorage.setItem(key, JSON.stringify(p))
  } catch {
    // kept until the page reloads
  }
}

export type PersonaState = { persona: Persona; save: (p: Persona) => Promise<void> }

export const PersonaContext = createContext<PersonaState>({ persona: defaultPersona, save: async () => {} })

export const usePersona = () => useContext(PersonaContext)

// The language the coach answers in: the persona's, or the UI's for auto.
export const coachLang = (p: Persona, uiLocale: string) => (p.language === 'auto' ? uiLocale : p.language)
