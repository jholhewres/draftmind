import { useEffect, useMemo, useRef, useState } from 'react'
import { fetchGsiStatus, fetchHeroes, fetchMatch, heroImageBase, type Hero, type MatchMessage, type MatchSession } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import BuildCard from './BuildCard'
import CoachChat from './CoachChat'
import History from './History'
import LiveCoach from './LiveCoach'
import MatchEnd from './MatchEnd'
import MatchTimer from './MatchTimer'
import type { CoachView } from '../shell/route'
import { saveSessionId, savedSessionId } from './session'

const position: Record<number, MessageKey> = {
  1: 'draft.position.1',
  2: 'draft.position.2',
  3: 'draft.position.3',
  4: 'draft.position.4',
  5: 'draft.position.5',
}

// How often the match is read while the game posts its state.
const GSI_POLL_MS = 8000

type State = { status: 'loading' } | { status: 'none' } | { status: 'error'; error: string } | { status: 'ready'; session: MatchSession }

// The Coach tab: the match the player started on the Draft tab, restored
// from the id kept in localStorage after a reload; view opens its end or
// the player's history.
export default function CoachTab({ view, signedIn }: { view?: CoachView; signedIn: boolean }) {
  const { t } = useI18n()
  const [id] = useState(savedSessionId)
  const [state, setState] = useState<State>(id ? { status: 'loading' } : { status: 'none' })
  const [heroes, setHeroes] = useState<Hero[]>([])
  const [live, setLive] = useState<MatchMessage[]>([])

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then((c) => setHeroes(c.heroes))
      .catch(() => {})
    return () => ctrl.abort()
  }, [])

  useEffect(() => {
    if (!id) return
    const ctrl = new AbortController()
    fetchMatch(id, ctrl.signal)
      .then((session) => {
        if (session) return setState({ status: 'ready', session })
        saveSessionId(null) // gone from the server: start afresh
        setState({ status: 'none' })
      })
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setState({ status: 'error', error: e instanceof Error ? e.message : String(e) })
      })
    return () => ctrl.abort()
  }, [id])

  // With the game posting its state (ADR-0015), the live lines its code
  // triggers store are fetched every few seconds, and the match ends by itself.
  const readyId = state.status === 'ready' && !state.session.result ? state.session.id : null
  // Every message id the page shows: from the load, and live lines pushed since.
  const shown = useRef(new Set<number>())
  useEffect(() => {
    if (state.status === 'ready') state.session.messages.forEach((m) => shown.current.add(m.id))
    live.forEach((m) => shown.current.add(m.id))
  }, [state, live])
  useEffect(() => {
    if (!readyId || !signedIn) return
    const ctrl = new AbortController()
    const poll = async () => {
      const gsi = await fetchGsiStatus(ctrl.signal).catch(() => null)
      if (!gsi?.receiving) return
      const s = await fetchMatch(readyId, ctrl.signal).catch(() => null)
      if (!s) return
      // Lines the page already shows, from the load or the shared window, are skipped.
      const fresh = s.messages.filter((m) => m.source === 'live' && !shown.current.has(m.id))
      fresh.forEach((m) => shown.current.add(m.id))
      if (fresh.length) setLive((cur) => [...cur, ...fresh])
      if (s.result) setState({ status: 'ready', session: s })
    }
    void poll()
    const timer = window.setInterval(() => void poll(), GSI_POLL_MS)
    return () => {
      ctrl.abort()
      window.clearInterval(timer)
    }
  }, [readyId, signedIn])

  const byId = useMemo(() => new Map(heroes.map((h) => [h.id, h])), [heroes])
  if (view === 'history') return <History byId={byId} />
  if (state.status === 'ready' && view === 'end') {
    const s = state.session
    return (
      <MatchEnd
        key={s.id}
        session={s}
        heroName={byId.get(s.heroId)?.name ?? `#${s.heroId}`}
        signedIn={signedIn}
        onEnded={(result) => setState({ status: 'ready', session: { ...s, result } })}
      />
    )
  }
  // An ended match is over: the tab waits for the next one.
  if (state.status !== 'ready' || state.session.result) {
    return (
      <section className="screen">
        <header className="screen-head">
          <h1>{t('nav.coach')}</h1>
        </header>
        {state.status === 'loading' ? (
          <p role="status">{t('coach.loading')}</p>
        ) : state.status === 'error' ? (
          <p role="alert">{t('coach.loadError', { error: state.error })}</p>
        ) : (
          <p className="muted">{t('coach.empty')}</p>
        )}
        <a className="end-link" href="#/coach/history">
          {t('history.title')}
        </a>
        <a className="end-link" href="#/settings">
          {t('persona.title')}
        </a>
      </section>
    )
  }
  const s = state.session
  const hero = byId.get(s.heroId)
  return (
    <section className="screen coach">
      <header className="coach-head">
        {hero && <img src={heroImageBase + hero.img} alt="" width={128} height={72} />}
        <div className="coach-title">
          <h1>{t('coach.title', { hero: hero?.name ?? `#${s.heroId}`, position: t(position[s.role] ?? 'nav.coach') })}</h1>
          <MatchTimer id={s.id} />
        </div>
        <div className="coach-actions">
          <a className="settings-entry" href="#/settings" aria-label={t('persona.title')}>
            <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" />
            </svg>
          </a>
          <a className="chip end-match" href="#/coach/end">
            {t('end.open')}
          </a>
        </div>
      </header>
      <div className="coach-body">
        <LiveCoach session={s} onLine={(m) => setLive((cur) => [...cur, m])} />
        <BuildCard build={s.itemPlan} byId={byId} />
        <CoachChat session={s} pushed={live} />
      </div>
    </section>
  )
}
