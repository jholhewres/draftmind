import type { Phase, StoredBuild } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import PhaseList from '../coach/PhaseList'

const sourceLabel: Record<StoredBuild['source'], MessageKey> = {
  opendota: 'builds.source.opendota',
  curated: 'builds.source.curated',
  coach: 'builds.source.coach',
}

const phases: Phase[] = ['start', 'early', 'mid', 'late']

// The hero's stored builds, one panel per position, its items as cards.
export default function HeroBuilds({ builds }: { builds: StoredBuild[] }) {
  const { t, locale } = useI18n()
  const pct = new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 1 })
  const num = new Intl.NumberFormat(locale)
  return (
    <section className="panel hero-builds" aria-labelledby="hero-builds">
      <h2 id="hero-builds">{t('builds.title')}</h2>
      {builds.map((b) => {
        const label = t(`draft.position.${b.position}` as MessageKey)
        return (
          <article key={`${b.position}-${b.source}`} className="hero-build" aria-label={label}>
            <h3>
              {label}
              <span className="source-label">{t(sourceLabel[b.source])}</span>
            </h3>
            {b.games !== undefined && b.winRate !== undefined && (
              <p className="muted">{t('builds.record', { games: num.format(b.games), rate: pct.format(b.winRate) })}</p>
            )}
            <PhaseList
              phases={phases.map((phase) => ({ phase, items: b.items.filter((it) => it.phase === phase).map((it) => ({ ...it, count: 0 })) }))}
            />
          </article>
        )
      })}
    </section>
  )
}
