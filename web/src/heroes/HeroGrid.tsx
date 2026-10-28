import { useEffect, useMemo, useState } from 'react'
import { fetchHeroes, heroImageBase, type HeroCatalog } from '../api'
import { useI18n } from '../i18n'

export default function HeroGrid() {
  const { t } = useI18n()
  const [catalog, setCatalog] = useState<HeroCatalog | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')

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
    const heroes = catalog?.heroes ?? []
    return q ? heroes.filter((h) => h.name.toLocaleLowerCase().includes(q)) : heroes
  }, [catalog, query])

  if (error) return <p role="alert">{t('heroes.loadError', { error })}</p>
  if (!catalog) return <p role="status">{t('heroes.loading')}</p>

  return (
    <section className="heroes">
      <div className="heroes-bar">
        <input
          type="search"
          aria-label={t('heroes.search')}
          placeholder={t('heroes.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <span className="muted">{catalog.patch ? t('heroes.patch', { patch: catalog.patch }) : t('heroes.notSynced')}</span>
      </div>
      <ul className="hero-grid" aria-label={t('heroes.list')}>
        {visible.map((h) => (
          <li key={h.id} className="hero-card">
            <img src={heroImageBase + h.img} alt="" loading="lazy" width={128} height={72} />
            <span>{h.name}</span>
          </li>
        ))}
      </ul>
      {visible.length === 0 && <p className="muted">{t('heroes.noMatch', { query })}</p>}
    </section>
  )
}
