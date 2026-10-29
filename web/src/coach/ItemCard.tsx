import { useState } from 'react'
import { heroImageBase, type ItemCard as Item } from '../api'
import { useI18n } from '../i18n'

// An item from the database as a card: its image from the game's image CDN,
// name and cost. A missing or broken image falls back to the name's initial.
export default function ItemCard({ item, note, hot = false }: { item: Item; note?: string; hot?: boolean }) {
  const { t, locale } = useI18n()
  const [broken, setBroken] = useState(false)
  const src = item.img && !broken ? heroImageBase + item.img : ''
  return (
    <span className={hot ? 'item-card hot' : 'item-card'}>
      {src ? (
        <img src={src} alt="" width={44} height={32} loading="lazy" onError={() => setBroken(true)} />
      ) : (
        <span className="item-fallback" aria-hidden="true">
          {item.name.charAt(0)}
        </span>
      )}
      <span className="item-card-text">
        <span className="item-name">{item.name}</span>
        <span className="item-cost">{item.cost > 0 ? t('item.cost', { cost: new Intl.NumberFormat(locale).format(item.cost) }) : t('item.free')}</span>
        {note && <span className="item-note">{note}</span>}
      </span>
    </span>
  )
}
