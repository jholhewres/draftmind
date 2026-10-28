import { useEffect, useState } from 'react'
import { fetchHealth, type Health } from './api'
import HeroGrid from './heroes/HeroGrid'

export default function App() {
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHealth(ctrl.signal)
      .then(setHealth)
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [])

  return (
    <div className="app">
      <header>
        <h1>Draftmind</h1>
        <p className="muted">Draft help and an in-match coach, from a photo of the pick screen.</p>
      </header>
      <main>
        <HeroGrid />
      </main>
      <footer>
        <p className="status" role="status">
          API: {error ? `unreachable (${error})` : health ? `${health.status}, database ${health.database}` : 'checking…'}
        </p>
        <p>
          Draftmind is an independent tool, not affiliated with or endorsed by Valve Corporation. Dota 2 is a
          trademark of Valve. Draftmind runs on a separate device or tab and never reads, modifies or overlays the
          game client.
        </p>
      </footer>
    </div>
  )
}
