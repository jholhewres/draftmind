import { useEffect, useState } from 'react'
import { fetchSuggestions, type DraftInput, type SuggestResult } from '../api'

export type Ranking = { result: SuggestResult | null; error: string | null }

// The ranking of input, asked again on every change; the last list stays
// until the new one arrives, so a change re-ranks in place. A null input
// asks nothing.
export function useRanking(input: DraftInput | null): Ranking {
  const [result, setResult] = useState<SuggestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    if (!input) return
    const ctrl = new AbortController()
    fetchSuggestions(input, ctrl.signal)
      .then((r) => {
        setResult(r)
        setError(null)
      })
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [input])
  return { result, error }
}
