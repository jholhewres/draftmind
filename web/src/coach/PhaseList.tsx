import type { PhaseItems } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

const phaseLabel: Record<PhaseItems['phase'], MessageKey> = {
  start: 'build.phase.start',
  early: 'build.phase.early',
  mid: 'build.phase.mid',
  late: 'build.phase.late',
}

// Core items by game phase as chips; empty phases are left out.
export default function PhaseList({ phases, counts = false }: { phases: PhaseItems[]; counts?: boolean }) {
  const { t, locale } = useI18n()
  const num = new Intl.NumberFormat(locale)
  return (
    <dl className="phases">
      {phases
        .filter((p) => p.items.length > 0)
        .map((p) => (
          <div key={p.phase} className="phase">
            <dt>{t(phaseLabel[p.phase])}</dt>
            <dd>
              <ul className="item-chips" aria-label={t(phaseLabel[p.phase])}>
                {p.items.map((it) => (
                  <li key={it.id} className="item-chip" title={counts ? t('build.games', { count: num.format(it.count) }) : undefined}>
                    {it.name}
                  </li>
                ))}
              </ul>
            </dd>
          </div>
        ))}
    </dl>
  )
}
