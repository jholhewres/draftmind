import { useState } from 'react'
import { startMatch, type DraftInput } from '../api'
import { saveSessionId } from './session'

// Starts a match for the picked hero and the draft around it, keeps its id
// and opens the Coach tab.
export function useStartMatch(draft: DraftInput | null) {
  const [starting, setStarting] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const start = (hero: number) => {
    if (!draft || starting !== null) return
    setStarting(hero)
    setError(null)
    startMatch({ hero, ...draft })
      .then((s) => {
        saveSessionId(s.id)
        window.location.hash = '#/coach'
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setStarting(null))
  }
  return { start, starting, error }
}
