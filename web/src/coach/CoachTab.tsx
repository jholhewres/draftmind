import { useEffect, useMemo, useState } from 'react'
import { fetchBuild, fetchHeroes, type Build, type Hero } from '../api'
import { useI18n } from '../i18n'
import BuildCard from './BuildCard'

// The lineup in the hash: #/coach?hero=<id>&vs=<id>,<id>.
function lineupFromHash(hash: string): { hero: number; enemies: number[] } | null {
  const query = hash.split('?')[1]
  if (!query) return null
  const q = new URLSearchParams(query)
  const hero = Number(q.get('hero'))
  if (!Number.isInteger(hero) || hero <= 0) return null
  const enemies = (q.get('vs') ?? '')
    .split(',')
    .filter(Boolean)
    .map(Number)
    .filter((n) => Number.isInteger(n) && n > 0)
  return { hero, enemies }
}

// The Coach tab: the locked hero and its build against the enemy lineup.
export default function CoachTab({ hash }: { hash: string }) {
  const { t } = useI18n()
  const lineup = useMemo(() => lineupFromHash(hash), [hash])
  const [heroes, setHeroes] = useState<Hero[]>([])
  const [build, setBuild] = useState<{ key: string; data?: Build; error?: string } | null>(null)
  const key = lineup ? `${lineup.hero}:${lineup.enemies.join(',')}` : ''

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then((c) => setHeroes(c.heroes))
      .catch(() => {})
    return () => ctrl.abort()
  }, [])

  useEffect(() => {
    if (!lineup) return
    const ctrl = new AbortController()
    fetchBuild(lineup.hero, lineup.enemies, ctrl.signal)
      .then((data) => setBuild({ key, data }))
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setBuild({ key, error: e instanceof Error ? e.message : String(e) })
      })
    return () => ctrl.abort()
  }, [lineup, key])

  const byId = useMemo(() => new Map(heroes.map((h) => [h.id, h])), [heroes])
  if (!lineup) {
    return (
      <section className="screen">
        <header className="screen-head">
          <h1>{t('nav.coach')}</h1>
        </header>
        <p className="muted">{t('coach.empty')}</p>
      </section>
    )
  }
  const current = build?.key === key ? build : null
  return (
    <section className="screen coach">
      <header className="screen-head">
        <h1>{byId.get(lineup.hero)?.name ?? t('nav.coach')}</h1>
      </header>
      {current?.error ? (
        <p role="alert">{t('build.error', { error: current.error })}</p>
      ) : current?.data ? (
        <BuildCard build={current.data} byId={byId} />
      ) : (
        <p role="status">{t('build.loading')}</p>
      )}
    </section>
  )
}
