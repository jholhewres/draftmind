import { heroImageBase, type FallbackReason, type Hero } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import { formatDelta, formatScore } from './format'
import type { Ranking } from './useRanking'

const SHOWN = 5

// What the fallback label says when hovered: why the code ranking stands.
const fallbackReason: Record<FallbackReason, MessageKey> = {
  timeout: 'draft.source.timeout',
  error: 'draft.source.error',
  unavailable: 'draft.source.unavailable',
  few_candidates: 'draft.source.fewCandidates',
}

type Props = Ranking & { byId: Map<number, Hero> }

// The top of the ranking with each hero's counter chips and score.
export default function SuggestionList({ result, error, byId }: Props) {
  const { t, locale } = useI18n()
  const name = (id: number) => byId.get(id)?.name ?? `#${id}`
  return (
    <section className="suggestions" aria-label={t('draft.suggestions')}>
      <div className="suggestions-head">
        <h2>{t('draft.suggestions')}</h2>
        {result?.source === 'ranking' && !error && (
          <span className="source-label" title={t(fallbackReason[result.reason ?? 'error'])}>
            {t('draft.source.ranking')}
          </span>
        )}
      </div>
      {error ? (
        <p role="alert">{t('draft.rankError', { error })}</p>
      ) : !result ? (
        <p role="status">{t('draft.ranking')}</p>
      ) : result.suggestions.length === 0 ? (
        <p className="muted">{t('draft.none')}</p>
      ) : (
        <ol className="suggestion-list" aria-label={t('draft.suggestions')}>
          {result.suggestions.slice(0, SHOWN).map((s) => {
            const hero = byId.get(s.heroId)
            return (
              <li key={s.heroId} className="suggestion">
                {hero && <img src={heroImageBase + hero.img} alt="" width={128} height={72} />}
                <div className="suggestion-main">
                  <span className="suggestion-name">{name(s.heroId)}</span>
                  {s.enemies.length > 0 && (
                    <span className="deltas">
                      {s.enemies.map((e) => {
                        const { arrow, text } = formatDelta(locale, e.delta)
                        const dir = arrow === '▲' ? 'up' : arrow === '▼' ? 'down' : 'even'
                        return (
                          <span key={e.heroId} className={`delta ${dir}`}>
                            {t('draft.delta', { arrow, hero: name(e.heroId), delta: text }).trim()}
                          </span>
                        )
                      })}
                    </span>
                  )}
                </div>
                <span className="score">
                  <span className="score-label">{t('draft.score')}</span>
                  <span className="score-value">{formatScore(locale, s.score)}</span>
                </span>
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
