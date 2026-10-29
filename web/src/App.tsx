import { useCallback, useEffect, useState } from 'react'
import { fetchAccount, fetchUILanguage, saveUILanguage, type Account } from './api'
import SignIn from './auth/SignIn'
import Landing from './landing/Landing'
import CoachTab from './coach/CoachTab'
import DraftTab from './draft/DraftTab'
import HeroGrid from './heroes/HeroGrid'
import HeroPage from './heroes/HeroPage'
import PatchesPage from './patches/PatchesPage'
import PersonaProvider from './settings/PersonaProvider'
import PersonaSettings from './settings/PersonaSettings'
import { isLocale, storedLocale, useI18n } from './i18n'
import { useRoute } from './shell/route'
import Footer from './shell/Footer'
import TabBar from './shell/TabBar'

export default function App() {
  const { t, setLocale } = useI18n()
  const { tab, hero, manual, signin, landing, coach, settings } = useRoute()
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

  // A signed-in player's saved UI language follows them to this device; one
  // picked here before they had saved any is kept as theirs.
  const signedIn = !!account
  useEffect(() => {
    if (!signedIn) return
    const ctrl = new AbortController()
    fetchUILanguage(ctrl.signal)
      .then((lang) => {
        if (isLocale(lang)) return setLocale(lang)
        const kept = storedLocale()
        if (kept) return saveUILanguage(kept)
      })
      .catch(() => {})
    return () => ctrl.abort()
  }, [signedIn, setLocale])

  if (signin) return <SignIn token={signin.token} failed={signin.failed} account={account} onAccount={onAccount} />
  // The root is the landing page for a visitor and Draft once signed in.
  if (landing && account === undefined) return null
  if (landing && account === null) return <Landing />

  return (
    <PersonaProvider account={account}>
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
            {settings ? (
              <PersonaSettings signedIn={!!account} />
            ) : hero ? (
              <HeroPage slug={hero} />
            ) : tab === 'heroes' ? (
              <HeroGrid />
            ) : tab === 'draft' ? (
              <DraftTab manual={manual} />
            ) : tab === 'coach' ? (
              <CoachTab view={coach} signedIn={!!account} />
            ) : (
              <PatchesPage />
            )}
          </main>
          <Footer signedIn={signedIn} />
        </div>
      </div>
    </PersonaProvider>
  )
}
