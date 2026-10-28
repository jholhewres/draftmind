import { useEffect, useState } from 'react'
import { fetchHealth, type Health } from './api'
import HeroGrid from './heroes/HeroGrid'
import { useI18n } from './i18n'

export default function App() {
  const { t } = useI18n()
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
        <p className="muted">{t('app.tagline')}</p>
      </header>
      <main>
        <HeroGrid />
      </main>
      <footer>
        <p className="status" role="status">
          {error
            ? t('app.health.unreachable', { error })
            : health
              ? t('app.health.ok', { status: health.status, database: health.database })
              : t('app.health.checking')}
        </p>
        <p>{t('app.trademark')}</p>
      </footer>
    </div>
  )
}
