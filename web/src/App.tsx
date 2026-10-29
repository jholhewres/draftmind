import { useEffect, useState } from 'react'
import { fetchHealth, type Health } from './api'
import CoachTab from './coach/CoachTab'
import DraftTab from './draft/DraftTab'
import HeroGrid from './heroes/HeroGrid'
import HeroPage from './heroes/HeroPage'
import PatchesPage from './patches/PatchesPage'
import { useI18n } from './i18n'
import { useRoute } from './shell/route'
import TabBar from './shell/TabBar'

export default function App() {
  const { t } = useI18n()
  const { tab, hero, manual } = useRoute()
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
      <TabBar current={tab} />
      <div className="column">
        <header className="brand">
          <span className="brand-mark" aria-hidden="true">
            D
          </span>
          <span className="brand-name">Draftmind</span>
        </header>
        <main>
          {hero ? (
            <HeroPage slug={hero} />
          ) : tab === 'heroes' ? (
            <HeroGrid />
          ) : tab === 'draft' ? (
            <DraftTab manual={manual} />
          ) : tab === 'coach' ? (
            <CoachTab />
          ) : (
            <PatchesPage />
          )}
        </main>
        <footer>
          <p className="note">{t('app.trademark')}</p>
          <p className="status" role="status">
            {error
              ? t('app.health.unreachable', { error })
              : health
                ? t('app.health.ok', { status: health.status, database: health.database })
                : t('app.health.checking')}
          </p>
        </footer>
      </div>
    </div>
  )
}
