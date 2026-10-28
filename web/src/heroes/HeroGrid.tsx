import { useEffect, useMemo, useState } from 'react'
import { fetchHeroes, heroImageBase, type HeroCatalog } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

// Primary attributes as the API names them; "all" is Universal.
const attrs = ['str', 'agi', 'int', 'all'] as const
type Attr = (typeof attrs)[number]

const attrLabel: Record<Attr, MessageKey> = {
  str: 'heroes.attr.str',
  agi: 'heroes.attr.agi',
  int: 'heroes.attr.int',
  all: 'heroes.attr.all',
}

export default function HeroGrid() {
  const { t } = useI18n()
  const [catalog, setCatalog] = useState<HeroCatalog | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [attr, setAttr] = useState<Attr | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then(setCatalog)
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [])

  const visible = useMemo(() => {
    const q = query.trim().toLocaleLowerCase()
    return (catalog?.heroes ?? []).filter(
      (h) => (!attr || h.primaryAttr === attr) && (!q || h.name.toLocaleLowerCase().includes(q)),
    )
  }, [catalog, query, attr])

  return (
    <section className="screen">
      <header className="screen-head">
        <h1>{t('nav.heroes')}</h1>
        {catalog && <span className="muted">{catalog.patch ? t('heroes.patch', { patch: catalog.patch }) : t('heroes.notSynced')}</span>}
      </header>
      {error ? (
        <p role="alert">{t('heroes.loadError', { error })}</p>
      ) : !catalog ? (
        <p role="status">{t('heroes.loading')}</p>
      ) : (
        <>
          <label className="search">
            <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="7" />
              <path d="M21 21l-4.3-4.3" />
            </svg>
            <input
              type="search"
              aria-label={t('heroes.search')}
              placeholder={t('heroes.search')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <div className="attr-filter" role="group" aria-label={t('heroes.attr.label')}>
            <button type="button" className="chip" aria-pressed={attr === null} onClick={() => setAttr(null)}>
              {t('heroes.attr.any')}
            </button>
            {attrs.map((a) => (
              <button key={a} type="button" className="chip" aria-pressed={attr === a} aria-label={t(attrLabel[a])} onClick={() => setAttr(a)}>
                <span className={`attr-icon attr-${a}`} aria-hidden="true" />
              </button>
            ))}
          </div>
          <ul className="hero-grid" aria-label={t('heroes.list')}>
            {visible.map((h) => (
              <li key={h.id} className="hero-card">
                <img src={heroImageBase + h.img} alt="" loading="lazy" width={128} height={72} />
                <span className="hero-name">{h.name}</span>
              </li>
            ))}
          </ul>
          {visible.length === 0 && <p className="muted">{t('heroes.noMatch', { query })}</p>}
        </>
      )}
    </section>
  )
}
