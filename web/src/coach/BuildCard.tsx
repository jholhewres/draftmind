import type { Build, Hero, Urgency } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import ItemCard from './ItemCard'
import PhaseList from './PhaseList'

const urgencyLabel: Record<Urgency, MessageKey> = {
  now: 'build.urgency.now',
  soon: 'build.urgency.soon',
  later: 'build.urgency.later',
}

// The locked hero's build: core items by phase from item popularity, then
// the situational items, each naming the enemy hero it answers.
export default function BuildCard({ build, byId }: { build: Omit<Build, 'patch' | 'popularitySource'>; byId: Map<number, Hero> }) {
  const { t } = useI18n()
  const name = (id: number) => byId.get(id)?.name ?? `#${id}`
  const afterStart = build.core.filter((p) => p.phase !== 'start')
  return (
    <section className="panel build-card" aria-labelledby="build-title">
      <div className="build-head">
        <h2 id="build-title">{t('build.title')}</h2>
        {build.source === 'ranking' && <span className="source-label">{t('build.ranked')}</span>}
      </div>
      {build.situational.length > 0 && (
        <ul className="item-cards situational" aria-label={t('build.situational')}>
          {build.situational.map((s) => (
            <li key={s.item.id} className={s.urgency ? `urgency-${s.urgency}` : undefined}>
              <ItemCard
                item={s.item}
                hot
                note={s.urgency ? t('build.vsEnemyUrgent', { urgency: t(urgencyLabel[s.urgency]), enemy: name(s.enemyId) }) : t('build.vsEnemy', { enemy: name(s.enemyId) })}
              />
            </li>
          ))}
        </ul>
      )}
      {afterStart.some((p) => p.items.length > 0) ? <PhaseList phases={afterStart} /> : <p className="muted">{t('build.none')}</p>}
      <p className="muted build-source">{t('build.source')}</p>
    </section>
  )
}
