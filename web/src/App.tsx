import { useEffect, useState } from 'react'
import { fetchHealth, type Health } from './api'
import HeroGrid from './heroes/HeroGrid'
import { useI18n } from './i18n'
import Placeholder from './shell/Placeholder'
import { tabLabel, useTab } from './shell/route'
import TabBar from './shell/TabBar'

export default function App() {
  const { t } = useI18n()
  const tab = useTab()
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
        <main>{tab === 'heroes' ? <HeroGrid /> : <Placeholder title={tabLabel[tab]} />}</main>
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
