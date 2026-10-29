import { useEffect, useRef, useState, type FormEvent, type MouseEvent } from 'react'
import {
  fetchSignInMethods,
  openSignInLink,
  PasswordError,
  registerWithPassword,
  requestSignInLink,
  signInWithPassword,
  signOut,
  steamSignInPath,
  type Account,
} from '../api'
import { useI18n } from '../i18n'

type Props = {
  token: string | null
  failed: boolean
  // undefined while the session is checked, null for a visitor.
  account: Account | null | undefined
  onAccount: (a: Account | null) => void
}

type Link = { status: 'idle' } | { status: 'opening' } | { status: 'used' } | { status: 'error'; error: string }

type Pass = { status: 'idle' } | { status: 'busy'; action: 'signin' | 'register' } | { status: 'error'; error: string }

type Send = { status: 'idle' } | { status: 'sending' } | { status: 'sent'; email: string; minutes: number } | { status: 'error'; error: string }

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Sign in with an e-mail and password, Steam, or an e-mailed link when the
// server has a mailer (ADR-0008, ADR-0010), after the prototype's Login
// screen; signed in, the account and a way out. An e-mailed link opens
// #/signin/link/<token>, which is spent here and dropped from the address
// bar.
export default function SignIn({ token, failed, account, onAccount }: Props) {
  const { t, locale } = useI18n()
  const [link, setLink] = useState<Link>({ status: token ? 'opening' : 'idle' })
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [pass, setPass] = useState<Pass>({ status: 'idle' })
  // Off until the server says it has a mailer.
  const [emailLink, setEmailLink] = useState(false)
  const [send, setSend] = useState<Send>({ status: 'idle' })
  const [signOutError, setSignOutError] = useState<string | null>(null)
  // The token already posted: StrictMode runs effects twice, and a second
  // post would find the link spent.
  const opened = useRef<string | null>(null)

  useEffect(() => {
    if (!token || opened.current === token) return
    opened.current = token
    setLink({ status: 'opening' })
    openSignInLink(token)
      .then((a) => {
        if (!a) return setLink({ status: 'used' })
        setLink({ status: 'idle' })
        onAccount(a)
        window.location.replace('#/signin')
      })
      .catch((e: unknown) => setLink({ status: 'error', error: message(e) }))
  }, [token, onAccount])

  useEffect(() => {
    const ctl = new AbortController()
    fetchSignInMethods(ctl.signal)
      .then((m) => setEmailLink(m.emailLink))
      .catch(() => setEmailLink(false))
    return () => ctl.abort()
  }, [])

  const passwordError = (err: unknown) => {
    if (!(err instanceof PasswordError)) return t('signin.passwordFailed', { error: message(err) })
    if (err.reason === 'rate') return t('signin.tooMany')
    if (err.reason === 'invalid') return t('signin.passwordRules', { min: 8, max: 200 })
    return t('signin.badLogin')
  }

  const signInOrRegister = (action: 'signin' | 'register') => {
    setPass({ status: 'busy', action })
    const go = action === 'register' ? registerWithPassword : signInWithPassword
    go(email.trim(), password)
      .then((a) => {
        setPass({ status: 'idle' })
        setPassword('')
        onAccount(a)
      })
      .catch((err: unknown) => setPass({ status: 'error', error: passwordError(err) }))
  }

  const submitPassword = (e: FormEvent) => {
    e.preventDefault()
    signInOrRegister('signin')
  }

  const createAccount = (e: MouseEvent<HTMLButtonElement>) => {
    // The browser's checks (e-mail, 8 to 200 characters) apply to both buttons.
    const form = e.currentTarget.form
    if (form && !form.reportValidity()) return
    signInOrRegister('register')
  }

  const sendLink = () => {
    const to = email.trim()
    setSend({ status: 'sending' })
    requestSignInLink(to, locale)
      .then((minutes) => setSend({ status: 'sent', email: to, minutes }))
      .catch((err: unknown) => setSend({ status: 'error', error: message(err) }))
  }

  const leave = () => {
    setSignOutError(null)
    signOut()
      .then(() => {
        setSend({ status: 'idle' })
        onAccount(null)
      })
      .catch((e: unknown) => setSignOutError(message(e)))
  }

  const checking = account === undefined || link.status === 'opening'

  return (
    <div className="signin-screen">
      <header className="signin-top">
        <a className="back" href="#/" aria-label={t('signin.back')}>
          <svg aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M19 12H5M12 19l-7-7 7-7" />
          </svg>
        </a>
      </header>
      <main className="signin">
        <div className="signin-head">
          <div className="signin-mark" aria-hidden="true">
            D
          </div>
          <h1>{account ? t('signin.accountTitle') : t('signin.title')}</h1>
          {!account && <p className="muted lead">{t('signin.lead')}</p>}
        </div>
        {checking ? (
          <p className="muted signin-status" role="status">
            {link.status === 'opening' ? t('signin.opening') : t('signin.checking')}
          </p>
        ) : account ? (
          <div className="panel account">
            <p>{account.email ? t('signin.asEmail', { email: account.email }) : t('signin.asSteam', { steamId: account.steamId ?? '' })}</p>
            <button type="button" className="option" onClick={leave}>
              {t('signin.signOut')}
            </button>
            {signOutError && <p role="alert">{t('signin.signOutError', { error: signOutError })}</p>}
          </div>
        ) : (
          <>
            {failed && <p role="alert">{t('signin.steamFailed')}</p>}
            {link.status === 'used' && <p role="alert">{t('signin.linkUsed')}</p>}
            {link.status === 'error' && <p role="alert">{t('signin.openError', { error: link.error })}</p>}
            <form className="signin-form" onSubmit={submitPassword}>
              <label>
                {t('signin.email')}
                <input
                  type="email"
                  required
                  autoComplete="email"
                  placeholder={t('signin.emailPlaceholder')}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </label>
              <label>
                {t('signin.password')}
                <input
                  type="password"
                  required
                  minLength={8}
                  maxLength={200}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </label>
              <div className="signin-actions">
                <button type="submit" className="primary" disabled={pass.status === 'busy'}>
                  {pass.status === 'busy' && pass.action === 'signin' ? t('signin.signingIn') : t('signin.passwordSignIn')}
                </button>
                <button type="button" className="option" disabled={pass.status === 'busy'} onClick={createAccount}>
                  {pass.status === 'busy' && pass.action === 'register' ? t('signin.creating') : t('signin.register')}
                </button>
              </div>
              {pass.status === 'error' ? (
                <p role="alert">{pass.error}</p>
              ) : (
                <p className="muted hint">{t('signin.passwordHint', { min: 8 })}</p>
              )}
            </form>
            <div className="signin-or" aria-hidden="true">
              <span />
              {t('signin.or')}
              <span />
            </div>
            <a className="option steam" href={steamSignInPath}>
              <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <circle cx="15" cy="9" r="3" />
                <path d="M2.5 14.5l6 2.5M8.5 17a2.5 2.5 0 1 0 2.4-3.2" />
              </svg>
              {t('signin.steam')}
            </a>
            {emailLink && (
              <div className="signin-link">
                <button type="button" className="option" disabled={send.status === 'sending' || !email.trim()} onClick={sendLink}>
                  {send.status === 'sending' ? t('signin.sending') : t('signin.send')}
                </button>
                {send.status === 'sent' ? (
                  <p className="sent" role="status">
                    {t('signin.sent', { email: send.email, minutes: send.minutes })}
                  </p>
                ) : send.status === 'error' ? (
                  <p role="alert">{t('signin.sendError', { error: send.error })}</p>
                ) : (
                  <p className="muted hint">{t('signin.hint')}</p>
                )}
              </div>
            )}
          </>
        )}
      </main>
      <footer className="signin-foot">{t('app.trademark')}</footer>
    </div>
  )
}
