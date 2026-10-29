import { useEffect, useId, useMemo, useState } from 'react'
import { fetchHeroes, fetchPatch, fetchPatches, heroImageBase, type Hero, type PatchChanges, type PatchSummary } from '../api'
import { useI18n } from '../i18n'
import NoteList from './NoteList'

type Load<T> = { status: 'loading' } | { status: 'error'; error: string } | { status: 'ready'; data: T }

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// The Patches tab: pick a patch, see what it changed for heroes and items,
// and narrow the heroes to one.
export default function PatchesPage() {
  const { t, locale } = useI18n()
  const filterId = useId()
  const [list, setList] = useState<Load<PatchSummary[]>>({ status: 'loading' })
  const [heroes, setHeroes] = useState<Hero[]>([])
  const [chosen, setChosen] = useState<string | null>(null)
  const [heroFilter, setHeroFilter] = useState('')
  const [patch, setPatch] = useState<{ key: string } & Load<PatchChanges>>({ key: '', status: 'loading' })

  useEffect(() => {
    const ctrl = new AbortController()
    fetchPatches(ctrl.signal)
      .then((r) => setList({ status: 'ready', data: Array.isArray(r.patches) ? r.patches : [] }))
      .catch((e: unknown) => !ctrl.signal.aborted && setList({ status: 'error', error: message(e) }))
    fetchHeroes(ctrl.signal)
      .then((r) => setHeroes(r.heroes))
      .catch(() => {}) // the filter only lists the heroes of the patch then
    return () => ctrl.abort()
  }, [])

  const patches = list.status === 'ready' ? list.data : []
  const name = chosen ?? patches[0]?.name ?? null
  const key = name ? `${name}:${locale}` : ''

  useEffect(() => {
    if (!name) return
    const ctrl = new AbortController()
    fetchPatch(name, locale, ctrl.signal)
      .then((data) => setPatch({ key: `${name}:${locale}`, status: 'ready', data }))
      .catch((e: unknown) => !ctrl.signal.aborted && setPatch({ key: `${name}:${locale}`, status: 'error', error: message(e) }))
    return () => ctrl.abort()
  }, [name, locale])

  // Every hero of the current patch, else the heroes this patch changed.
  const options = useMemo(() => {
    const byId = new Map<number, Hero>(heroes.map((h) => [h.id, h]))
    if (patch.status === 'ready') for (const h of patch.data.heroes) if (!byId.has(h.id)) byId.set(h.id, h)
    return [...byId.values()].sort((a, b) => a.name.localeCompare(b.name, locale))
  }, [heroes, patch, locale])

  const date = (iso: string) => new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(iso))
  const summary = patches.find((p) => p.name === name)
  const current = patch.key === key ? patch : ({ status: 'loading' } as const)
  const shown = current.status === 'ready' ? current.data.heroes.filter((h) => !heroFilter || h.slug === heroFilter) : []
  const filterName = options.find((h) => h.slug === heroFilter)?.name ?? heroFilter

  return (
    <section className="screen patches">
      <header className="screen-head">
        <h1>{t('nav.patches')}</h1>
      </header>
      {list.status === 'loading' ? (
        <p role="status">{t('patches.loading')}</p>
      ) : list.status === 'error' ? (
        <p role="alert">{t('patches.loadError', { error: list.error })}</p>
      ) : patches.length === 0 ? (
        <p className="muted">{t('patches.none')}</p>
      ) : (
        <>
          <div className="patch-picker" role="group" aria-label={t('patches.choose')}>
            {patches.map((p, i) => (
              <button key={p.name} type="button" className="chip" aria-pressed={p.name === name} onClick={() => setChosen(p.name)}>
                {i === 0 ? t('patches.latest', { patch: p.name }) : p.name}
              </button>
            ))}
          </div>
          {summary && (
            <div className="panel patch-summary">
              <strong>{t('patches.title', { patch: summary.name })}</strong>
              <span className="muted">
                {t('patches.summary', { heroes: summary.heroes, items: summary.items, date: date(summary.releasedAt) })}
              </span>
            </div>
          )}
          <div className="hero-filter">
            <label htmlFor={filterId}>{t('patches.filter')}</label>
            <select id={filterId} value={heroFilter} onChange={(e) => setHeroFilter(e.target.value)}>
              <option value="">{t('patches.allHeroes')}</option>
              {options.map((h) => (
                <option key={h.id} value={h.slug}>
                  {h.name}
                </option>
              ))}
            </select>
          </div>
          {current.status === 'loading' ? (
            <p role="status">{t('patches.loadingPatch')}</p>
          ) : current.status === 'error' ? (
            <p role="alert">{t('patches.loadError', { error: current.error })}</p>
          ) : (
            <div className="patch-body">
              {current.data.general.length > 0 && !heroFilter && (
                <section className="patch-section" aria-labelledby="patch-general">
                  <h2 id="patch-general">{t('patches.general')}</h2>
                  <div className="panel">
                    <NoteList notes={current.data.general} />
                  </div>
                </section>
              )}
              <section className="patch-section" aria-labelledby="patch-heroes">
                <h2 id="patch-heroes">{t('patches.heroes')}</h2>
                {shown.length === 0 ? (
                  <p className="muted">{heroFilter ? t('patches.noHeroChange', { hero: filterName }) : t('patches.noHeroes')}</p>
                ) : (
                  <ul className="patch-heroes" aria-label={t('patches.heroes')}>
                    {shown.map((h) => (
                      <li key={h.id} className="panel patch-hero">
                        <a href={`#/heroes/${encodeURIComponent(h.slug)}`} className="patch-hero-head">
                          <img src={heroImageBase + h.img} alt="" width={128} height={72} />
                          <h3>{h.name}</h3>
                        </a>
                        <NoteList notes={h.notes} />
                      </li>
                    ))}
                  </ul>
                )}
              </section>
              {!heroFilter && (
                <section className="patch-section" aria-labelledby="patch-items">
                  <h2 id="patch-items">{t('patches.items')}</h2>
                  {current.data.items.length === 0 ? (
                    <p className="muted">{t('patches.noItems')}</p>
                  ) : (
                    <ul className="patch-items" aria-label={t('patches.items')}>
                      {current.data.items.map((it) => (
                        <li key={it.id} className="panel">
                          <h3>{it.name}</h3>
                          <NoteList notes={it.notes} />
                        </li>
                      ))}
                    </ul>
                  )}
                </section>
              )}
            </div>
          )}
        </>
      )}
    </section>
  )
}
