import { createContext, useContext } from 'react'
import { defaultPersona, type Persona } from '../api'

// A visitor's persona lives in this browser; a signed-in player's on the server.
const key = 'draftmind.persona'

export function storedPersona(): Persona {
  try {
    const p = JSON.parse(window.localStorage.getItem(key) ?? '') as Partial<Persona>
    return { ...defaultPersona, ...p }
  } catch {
    return defaultPersona
  }
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
