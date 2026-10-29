import { useEffect, useState } from 'react'
import { fetchMyMatches, heroImageBase, type Hero, type PastMatch } from '../api'
import { useI18n } from '../i18n'
import BackButton from '../shell/BackButton'
import { clock } from './clock'

type State = { status: 'loading' } | { status: 'visitor' } | { status: 'error' } | { status: 'ready'; matches: PastMatch[] }

// The player's ended matches, newest first: hero, result and date. A
// visitor is asked to sign in to keep one.
export default function History({ byId }: { byId: Map<number, Hero> }) {
  const { t, locale } = useI18n()
  const [state, setState] = useState<State>({ status: 'loading' })

  useEffect(() => {
    const ctrl = new AbortController()
    fetchMyMatches(ctrl.signal)
      .then((matches) => setState(matches ? { status: 'ready', matches } : { status: 'visitor' }))
      .catch(() => {
        if (!ctrl.signal.aborted) setState({ status: 'error' })
      })
    return () => ctrl.abort()
  }, [])

  const date = new Intl.DateTimeFormat(locale, { dateStyle: 'medium' })
  return (
    <section className="screen match-history">
      <header className="screen-head with-back">
        <BackButton parent="#/coach" label={t('end.back')} />
        <h1>{t('history.title')}</h1>
      </header>
      {state.status === 'loading' ? (
        <p role="status">{t('history.loading')}</p>
      ) : state.status === 'visitor' ? (
        <p className="sign-in-prompt">
          {t('history.visitor')} <a href="#/signin">{t('account.signIn')}</a>
        </p>
      ) : state.status === 'error' ? (
        <p role="alert">{t('history.error')}</p>
      ) : state.matches.length === 0 ? (
        <p className="muted">{t('history.empty')}</p>
      ) : (
        <ol className="history-list" aria-label={t('history.title')}>
          {state.matches.map((m) => {
            const hero = byId.get(m.heroId)
            return (
              <li key={m.id}>
                {hero && <img src={heroImageBase + hero.img} alt="" width={64} height={36} />}
                <span className="history-hero">{hero?.name ?? `#${m.heroId}`}</span>
                <span className={`history-result ${m.result}`}>{t(m.result === 'win' ? 'end.win' : 'end.loss')}</span>
                <span className="muted history-meta">
                  {date.format(new Date(m.endedAt))}
                  {m.durationS ? ` · ${clock(m.durationS)}` : ''}
                  {m.kills !== undefined ? ` · ${m.kills}/${m.deaths}/${m.assists}` : ''}
                </span>
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
