import { useEffect, useMemo, useRef, useState } from 'react'
import { heroImageBase, type Hero } from '../api'
import { useI18n } from '../i18n'

type Props = {
  title: string
  heroes: Hero[] // the heroes that may go in the slot
  filled: boolean
  onPick: (id: number) => void
  onRemove: () => void
  onClose: () => void
}

// A sheet over the board with a searchable portrait grid.
export default function HeroPicker({ title, heroes, filled, onPick, onRemove, onClose }: Props) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')
  const search = useRef<HTMLInputElement>(null)

  useEffect(() => search.current?.focus(), [])

  const visible = useMemo(() => {
    const q = query.trim().toLocaleLowerCase()
    return q ? heroes.filter((h) => h.name.toLocaleLowerCase().includes(q)) : heroes
  }, [heroes, query])

  return (
    <div className="picker-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div
        className="picker"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <div className="picker-head">
          <h2>{title}</h2>
          {filled && (
            <button type="button" className="chip" onClick={onRemove}>
              {t('draft.remove')}
            </button>
          )}
          <button type="button" className="picker-close" aria-label={t('draft.close')} onClick={onClose}>
            <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </button>
        </div>
        <label className="search">
          <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="7" />
            <path d="M21 21l-4.3-4.3" />
          </svg>
          <input
            ref={search}
            type="search"
            aria-label={t('heroes.search')}
            placeholder={t('heroes.search')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <ul className="picker-grid" aria-label={t('heroes.list')}>
          {visible.map((h) => (
            <li key={h.id}>
              <button type="button" onClick={() => onPick(h.id)}>
                <img src={heroImageBase + h.img} alt="" loading="lazy" width={128} height={72} />
                <span>{h.name}</span>
              </button>
            </li>
          ))}
        </ul>
        {visible.length === 0 && <p className="muted">{t('heroes.noMatch', { query: query.trim() })}</p>}
      </div>
    </div>
  )
}
