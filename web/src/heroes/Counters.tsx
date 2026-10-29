import { useEffect, useState } from 'react'
import { fetchCounters, heroImageBase, type Counter, type Counters as CountersData } from '../api'
import { useI18n } from '../i18n'

// The heroes this hero beats and the heroes that beat it on the current
// patch; nothing while it loads or when no matchups are synced.
export default function Counters({ slug }: { slug: string }) {
  const { t, locale } = useI18n()
  const [counters, setCounters] = useState<{ slug: string; data: CountersData | null } | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchCounters(slug, ctrl.signal)
      .then((data) => setCounters({ slug, data }))
      .catch(() => {}) // the page stands without them
    return () => ctrl.abort()
  }, [slug])

  const data = counters?.slug === slug ? counters.data : null
  if (!data || (data.goodAgainst.length === 0 && data.badAgainst.length === 0)) return null
  const delta = (n: number) =>
    new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1, signDisplay: 'exceptZero' }).format(n)
  const anyLow = [...data.goodAgainst, ...data.badAgainst].some((c) => c.lowSample)

  const column = (side: 'good' | 'bad', list: Counter[]) => (
    <div className={`counter-side ${side}`}>
      <h3 id={`counters-${side}`}>
        <span aria-hidden="true">{side === 'good' ? '▲' : '▼'}</span> {t(side === 'good' ? 'hero.goodAgainst' : 'hero.badAgainst')}
      </h3>
      <ul aria-labelledby={`counters-${side}`}>
        {list.map((c) => (
          <li key={c.id} className={c.lowSample ? 'low-sample' : undefined} title={t('hero.counterGames', { games: c.games })}>
            <a href={`#/heroes/${encodeURIComponent(c.slug)}`}>
              <img src={heroImageBase + c.img} alt="" width={128} height={72} />
              <span className="counter-name">{c.name}</span>
              <span className="counter-delta">{delta(c.delta)}</span>
              {c.lowSample && <span className="sr-only">{t('hero.lowSample')}</span>}
            </a>
          </li>
        ))}
      </ul>
    </div>
  )

  return (
    <section className="panel hero-counters" aria-labelledby="hero-counters">
      <h2 id="hero-counters">{t('hero.counters', { patch: data.patch })}</h2>
      <div className="counter-grid">
        {column('good', data.goodAgainst)}
        {column('bad', data.badAgainst)}
      </div>
      {anyLow && <p className="muted counter-note">{t('hero.lowSampleNote', { games: data.lowSampleGames })}</p>}
    </section>
  )
}
