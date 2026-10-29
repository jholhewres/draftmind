import { useEffect, useMemo, useState } from 'react'
import { fetchHeroes, fetchMatch, heroImageBase, type Hero, type MatchSession } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import BuildCard from './BuildCard'
import CoachChat from './CoachChat'
import History from './History'
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

type State = { status: 'loading' } | { status: 'none' } | { status: 'error'; error: string } | { status: 'ready'; session: MatchSession }

// The Coach tab: the match the player started on the Draft tab, restored
// from the id kept in localStorage after a reload; view opens its end or
// the player's history.
export default function CoachTab({ view, signedIn }: { view?: CoachView; signedIn: boolean }) {
  const { t } = useI18n()
  const [id] = useState(savedSessionId)
  const [state, setState] = useState<State>(id ? { status: 'loading' } : { status: 'none' })
  const [heroes, setHeroes] = useState<Hero[]>([])

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
        <a className="chip end-match" href="#/coach/end">
          {t('end.open')}
        </a>
      </header>
      <div className="coach-body">
        <BuildCard build={s.itemPlan} byId={byId} />
        <CoachChat session={s} />
      </div>
    </section>
  )
}
