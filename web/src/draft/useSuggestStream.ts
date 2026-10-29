import { useEffect, useRef, useState } from 'react'
import { streamSuggestions, type StreamRead, type StreamRequest, type SuggestResult, type Tip } from '../api'

// The list on screen: the code ranking first, refined in place by Jev's
// decision; deciding until the decision arrives.
export type StreamResult = SuggestResult & { deciding: boolean }

export type SuggestStream = {
  result: StreamResult | null
  error: string | null
  tips: ReadonlyMap<number, Tip>
  // The stream is still open: tips not yet here may still come.
  streaming: boolean
}

// Streams the suggestions for request, asked again whenever request
// changes. The last list and its tips stay until the new ranking arrives,
// so a change updates the suggestions in place. onRead gets a photo's read.
export function useSuggestStream(request: StreamRequest | null, onRead?: (read: StreamRead) => void): SuggestStream {
  const [result, setResult] = useState<StreamResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tips, setTips] = useState<ReadonlyMap<number, Tip>>(new Map())
  // The last request whose stream ended; any other is still streaming.
  const [ended, setEnded] = useState<StreamRequest | null>(null)
  const readRef = useRef(onRead)
  useEffect(() => {
    readRef.current = onRead
  })

  useEffect(() => {
    if (!request) return
    const ctrl = new AbortController()
    streamSuggestions(
      request,
      (ev) => {
        switch (ev.type) {
          case 'read':
            readRef.current?.(ev.read)
            break
          case 'ranking':
            setResult({ patch: ev.patch, source: 'ranking', suggestions: ev.suggestions, bans: ev.bans, stats: ev.stats, deciding: true })
            setTips(new Map())
            setError(null)
            break
          case 'decision':
            setResult((cur) => ({ patch: cur?.patch ?? '', bans: cur?.bans, stats: cur?.stats, ...ev.decision, deciding: false }))
            break
          case 'tip':
            setTips((cur) => new Map(cur).set(ev.tip.heroId, ev.tip))
            break
        }
      },
      ctrl.signal,
    )
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        if (ctrl.signal.aborted) return
        setEnded(request)
        // A stream cut before its decision keeps the ranking as it stands.
        setResult((cur) => (cur?.deciding ? { ...cur, deciding: false } : cur))
      })
    return () => ctrl.abort()
  }, [request])

  return { result, error, tips, streaming: request !== null && ended !== request }
}
