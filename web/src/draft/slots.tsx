import { useId, type ReactNode } from 'react'
import { heroImageBase, type Hero } from '../api'
import { useI18n } from '../i18n'
import { sideLabel, type Side } from './labels'

const plus = (
  <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
    <path d="M12 5v14M5 12h14" />
  </svg>
)

type SlotProps = {
  side: Side
  hero: Hero | undefined
  heroId: number | null
  // A slot the photo could not read: a highlighted + that asks for the hero.
  flagged?: boolean
  onClick: () => void
}

// One draft slot: the hero's portrait, or a + to add one.
export function SlotButton({ side, hero, heroId, flagged = false, onClick }: SlotProps) {
  const { t } = useI18n()
  const label = flagged
    ? t('photo.unread')
    : heroId === null
      ? t(sideLabel[side].add)
      : t(sideLabel[side].slot, { hero: hero?.name ?? `#${heroId}` })
  return (
    <button
      type="button"
      className={`slot ${flagged ? 'empty flagged' : heroId === null ? 'empty' : 'filled'}`}
      aria-label={label}
      onClick={onClick}
    >
      {hero && !flagged ? <img src={heroImageBase + hero.img} alt="" width={128} height={72} /> : plus}
    </button>
  )
}

// A labelled row of slots.
export function Row({ side, title, children }: { side: Side; title: string; children: ReactNode }) {
  const id = useId()
  return (
    <div className={`slot-row ${side}`} role="group" aria-labelledby={id}>
      <span id={id} className="row-label">
        {title}
      </span>
      <div className="slots">{children}</div>
    </div>
  )
}
