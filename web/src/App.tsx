import { useCallback, useEffect, useState } from 'react'
import { fetchAccount, fetchHealth, type Account, type Health } from './api'
import SignIn from './auth/SignIn'
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
  const { tab, hero, manual, signin } = useRoute()
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  // undefined while checked; a failed check reads as a visitor.
  const [account, setAccount] = useState<Account | null | undefined>(undefined)
  const onAccount = useCallback((a: Account | null) => setAccount(a), [])

  useEffect(() => {
    const ctrl = new AbortController()
    fetchAccount(ctrl.signal)
      .then(setAccount)
      .catch(() => {
        if (!ctrl.signal.aborted) setAccount(null)
      })
    return () => ctrl.abort()
  }, [])

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHealth(ctrl.signal)
      .then(setHealth)
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [])

  if (signin) return <SignIn token={signin.token} failed={signin.failed} account={account} onAccount={onAccount} />

  return (
    <div className="app">
      <TabBar current={tab} />
      <div className="column">
        <header className="brand">
          <span className="brand-mark" aria-hidden="true">
            D
          </span>
          <span className="brand-name">Draftmind</span>
          <a className="account-entry" href="#/signin" aria-label={account ? t('account.open') : t('account.signIn')}>
            <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="8" r="4" />
              <path d="M4 21a8 8 0 0 1 16 0" />
            </svg>
            <span>{account ? t('account.open') : t('account.signIn')}</span>
          </a>
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
